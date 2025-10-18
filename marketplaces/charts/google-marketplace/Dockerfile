FROM gcr.io/cloud-marketplace-tools/k8s/deployer_helm/onbuild

COPY schema-test.yaml /data-test/schema.yaml

# Replace deploy script with custom deploy script that enables kubectl --server-side apply
# kubectl --server-side apply is needed to prevent bloated `kubectl.kubernetes.io/last-applied-configuration`
# annotations on CRDs
COPY deploy_with_tests.sh /bin/deploy_with_tests.sh

RUN chmod +x /bin/deploy_with_tests.sh

ENTRYPOINT ["/bin/bash", "/bin/deploy.sh"]