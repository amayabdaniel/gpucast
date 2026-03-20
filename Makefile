.PHONY: build test run docker-build helm-install helm-uninstall

build:
	go build -o bin/gpucast .

test:
	go test ./... -v -count=1

run:
	go run . --listen=:9400

run-with-vllm:
	go run . --listen=:9400 --vllm-endpoint=http://localhost:8000/metrics --model=qwen3-8b

docker-build:
	docker build -t ghcr.io/amayabdaniel/gpucast:latest .

helm-install:
	helm upgrade --install gpucast deploy/helm/gpucast/ -n monitoring --create-namespace

helm-uninstall:
	helm uninstall gpucast -n monitoring
