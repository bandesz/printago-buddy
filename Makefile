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
	@echo "==> Building and pushing $(DOCKER_IMAGE):$(VERSION) for linux/amd64 and linux/arm64"
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t "$(DOCKER_IMAGE):$(VERSION)" \
		-t "$(DOCKER_IMAGE):latest" \
		--push \
		.
	@echo "==> Released v$(VERSION)"
