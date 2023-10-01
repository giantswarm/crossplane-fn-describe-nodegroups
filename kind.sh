#!/bin/bash
go generate ./...
docker build . -t docker.io/choclab/function-describe-nodegroups:v0.0.1
docker push choclab/function-describe-nodegroups:v0.0.1
readonly mc=honeybadgermc

export GITHUB_TOKEN=$(bwv "development/github.com?field=full-access-token-never-expire" | jq -r .value);

kind delete cluster -n xfn
kind create cluster --config ../examples/snail.yaml -n xfn
kubectl config use-context kind-xfn

helm repo add crossplane https://charts.crossplane.io/master/
# helm repo add control-plane-catalog https://giantswarm.github.io/control-plane-catalog/
helm repo update

# Requires AWS credentials to be set up with profile [snail]
eval $(awk -v mc=$mc '$0 ~ mc {x=NR+2; next; }(NR<=x){print "export "toupper($1)"="$3;}' ~/.aws/credentials)

# Requires aws config to be set up for profile [snail] IN ORDER
# ```
# [profile snail]
# region=<region>
# ```
eval $(awk -v mc=$mc '$0 ~ mc{x=NR+1; next; }(NR<=x){print "export AWS_"toupper($1)"="$3;}' ~/.aws/config)

export GOPROXY=off
# Install CAPI/CAPA - does not require stack initialisation
export AWS_B64ENCODED_CREDENTIALS=$(clusterawsadm bootstrap credentials encode-as-profile)

# export EXP_CLUSTER_RESOURCE_SET=true
export EXP_MACHINE_POOL=true
clusterctl init --infrastructure=aws:v2.2.2
cat <<EOF | k apply -f -
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: AWSClusterControllerIdentity
metadata:
  name: default
spec:
  allowedNamespaces:
    list: null
    selector: {}
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: AWSClusterRoleIdentity
metadata:
  labels:
    cluster.x-k8s.io/watch-filter: capi
    giantswarm.io/organization: giantswarm
  name: default
spec:
  allowedNamespaces:
    list: null
    selector: {}
  roleARN: arn:aws:iam::242036376510:role/giantswarm-snail-capa-controller
  sourceIdentityRef:
    kind: AWSClusterControllerIdentity
    name: default
EOF
echo "Installing irsa-operator"

# install IRSA operator...
mkdir irsa && {
  cd irsa;
  git init && git remote add -f origin git@github.com:giantswarm/irsa-operator.git &&
  git config core.sparseCheckout true && echo "helm/irsa-operator" >> .git/info/sparse-checkout &&
  git pull origin master;
  cd -;
}

kubectl create namespace giantswarm
helm install irsa-operator --namespace giantswarm ./irsa/helm/irsa-operator --set aws.accessKeyID=$AWS_ACCESS_KEY_ID,aws.secretAccessKey=$AWS_SECRET_ACCESS_KEY,region=$AWS_REGION,capa=true,legacy=false,installation.name=snail,global.podSecurityStandards.enforced=true

echo "Waiting for irsa-operator deployment to become ready"
until kubectl get deploy -n giantswarm irsa-operator -o yaml 2>/dev/null | yq '.status.conditions[] | select(.reason == "MinimumReplicasAvailable") .status' | grep -q True; do
    echo -n .
    sleep 1
done
echo
rm -rf irsa

cd ../amazon-eks-pod-identity-webhook
make cluster-up
cd ../function-describe-nodegroups

# Install crossplane
helm install crossplane --namespace crossplane --create-namespace crossplane-master/crossplane --devel
echo "Waiting for crossplane CRDs"
until grep -q functions <<<$(kubectl get crds 2>/dev/null); do
    echo -n .
    sleep 1
done
echo

# TODO: Ammend this to point at the secret containing your credentials
kubectl create secret generic aws-credentials -n crossplane --from-literal=creds="$(
  base64 -d <<< ${AWS_B64ENCODED_CREDENTIALS}
)"

kubectl apply -f ../examples/controllers.yaml
echo "Waiting for provider CRDs"
until grep -q 'providerconfigs.aws.upbound.io' <<<$(kubectl get crds 2>/dev/null) && grep -q 'providerconfigs.kubernetes.crossplane.io' <<<$(kubectl get crds 2>/dev/null); do
    echo -n .
    sleep 1
done
echo

kubectl apply -f ../examples/providerconfig-webident.yaml
kubectl apply -f ../examples/functions.yaml

# Wait for functions to become ready
until
    kubectl get functions function-generate-subnets -o yaml | yq '.status.conditions[] | select(.type == "Healthy" and .status == "True")' | grep -q "True" &&
        kubectl get functions function-generate-subnets -o yaml | yq '.status.conditions[] | select(.type == "Healthy" and .status == "True")' | grep -q "True" ;
do
    echo -n .
    sleep 1
done
echo

kubectl create namespace org-sample
kubectl apply -f ../examples/xrd
sleep 10
kubectl apply -f ../examples/claim.yaml
