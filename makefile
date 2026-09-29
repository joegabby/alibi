INSTALL_DIR = $(CURDIR)/bin
# Run on every code change
build:
	docker run --rm \
		-v "$(CURDIR):/app" \
		-v "$(INSTALL_DIR):/output" \
		-v "alibi-gomodcache:/go/pkg/mod" \
		-w /app \
		alibi-builder \
		bash -c "CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -ldflags='-X github.com/joegabby/alibi/cli/internal/cli.version=v1.0.0' -mod=mod -o /output/alibi.exe ./cli/cmd/cli"
		
# Run once, or when dependencies change
docker-setup:
	docker build -t alibi-builder .
	make build