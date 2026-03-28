#!/usr/bin/env bash
set -e

echo "Running chaind test suite..."
echo ""

if go test -race -count=1 -v ./...; then
    echo ""
    echo "========================================="
    echo "PASS — all tests passed"
    echo "========================================="
    exit 0
else
    echo ""
    echo "========================================="
    echo "FAIL — one or more tests failed"
    echo "========================================="
    exit 1
fi
