/bin/sh
- -c
- |
# Get all pods with label monitor="true"
PODS=$(kubectl get pods -l monitor="true" -o jsonpath='{.items[*].status.podIP}')
for POD_IP in $PODS; do
# Make HTTP request to /healthz endpoint
RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" http://$POD_IP/healthz)
if [ "$RESPONSE" -eq 200 ]; then
    curl -X POST "http://dba-service/increase_success?app_name=$app_name"
else
    curl -X POST "http://dba-service/increase_success?app_name=$app_name"
fi
done