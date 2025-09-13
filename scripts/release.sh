#!/bin/bash
# Based on https://go.dev/doc/modules/publishing

# Validate version
if [ -z "$1" ]; then
    echo "Error: Version number not provided. Please provide a version number formatted as major.minor.patch."
    exit 1
fi

# Check if the provided variable is a valid version number
if ! [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: Invalid version number format. Please provide a version number formatted as major.minor.patch."
    exit 1
fi

# Run go mod tidy and validate no files are changed
go mod tidy

if ! git diff --exit-code; then
    echo "Error: Changes detected in Git files. Please commit your changes before continuing."
    exit 1
fi

# Run tests and validate none failed
make test

if [ $? -ne 0 ]; then
    echo "Error: Tests failed. Please fix the issues before releasing."
    exit 1
fi

# Tag release
version="v$1"
git tag $version
git push origin $version

# Fetch new pkg
GOPROXY=proxy.golang.org go list -m github.com/failsafe-go/failsafe-go@$version
curl -s -o /dev/null https://pkg.go.dev/fetch/github.com/failsafe-go/failsafe-go@$version

if [ $? -eq 0 ]; then
    echo "Released successfully."
else
    echo "Error: Release failed."
    exit 1
fi