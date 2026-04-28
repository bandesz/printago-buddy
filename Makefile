DOCKER_IMAGE := bandesz/printago-buddy

.PHONY: build test lint release

build:
	go build -o bin/printago-buddy ./cmd/printago-buddy

test:
	go test ./...

lint:
	golangci-lint run ./...

## release VERSION=1.2.3  — tag, build, and push to Docker Hub
release:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make release VERSION=<semver>  e.g. make release VERSION=1.0.0"; \
		exit 1; \
	fi
	@echo "==> Tagging v$(VERSION)"
	git tag -a "v$(VERSION)" -m "Release v$(VERSION)"
	git push origin "v$(VERSION)"
	@echo "==> Building Docker image $(DOCKER_IMAGE):$(VERSION)"
	docker build \
		-t "$(DOCKER_IMAGE):$(VERSION)" \
		-t "$(DOCKER_IMAGE):latest" \
		.
	@echo "==> Pushing Docker images"
	docker push "$(DOCKER_IMAGE):$(VERSION)"
	docker push "$(DOCKER_IMAGE):latest"
	@echo "==> Released v$(VERSION)"
