#!/bin/bash
# file: scripts/fast-docker-build.sh
# Fast local Docker build with aggressive caching

set -e

echo "🚀 Starting optimized Docker build..."

# Check if we should use pre-built assets
USE_PREBUILT=${USE_PREBUILT:-false}
DOCKERFILE=${DOCKERFILE:-Dockerfile.optimized}

if [ "$USE_PREBUILT" = "true" ]; then
    echo "📦 Using pre-built assets from GitHub Packages..."
    DOCKERFILE="Dockerfile.fast"

    # Pull latest assets image
    docker pull ghcr.io/jdfalk/subtitle-manager/assets:latest || {
        echo "⚠️  Failed to pull pre-built assets, falling back to local build"
        DOCKERFILE="Dockerfile.optimized"
    }
fi

echo "🏗️  Building with $DOCKERFILE..."

# Enable BuildKit for better caching
export DOCKER_BUILDKIT=1

# Build with maximum cache utilization
docker build \
    --file "$DOCKERFILE" \
    --tag subtitle-manager:latest \
    --build-arg BUILDKIT_INLINE_CACHE=1 \
    --cache-from subtitle-manager:latest \
    --cache-from subtitle-manager:cache \
    .

echo "✅ Build completed successfully!"
echo "🔍 Image size:"
docker images subtitle-manager:latest --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}"

echo ""
echo "💡 To run the container:"
echo "docker run -p 8080:8080 -v \$(pwd)/config:/config subtitle-manager:latest"
