#!/bin/sh

# The script requires:
# - rockcraft
# - skopeo
# - yq
# - docker

set -e

echo "Building image: $IMAGE"

if [ -z "$IMAGE" ]; then
  IMAGE="tenant-service:latest"
fi

rockcraft clean
rockcraft pack -v

echo "$IMAGE built"

# Copy image to localhost registry
echo "Pushing to registry $IMAGE..."
skopeo --insecure-policy copy --dest-tls-verify=false \
  "oci-archive:tenant-service_$(yq -r '.version' rockcraft.yaml)_amd64.rock" \
  "docker://$IMAGE"
echo "$IMAGE pushed"
