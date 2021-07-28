default: testacc
version = 0.2.2
local_path = ~/.terraform.d/plugins/local/MathiasPius/zfs/$(version)/linux_amd64/

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

.PHONY: build-local
build-local:
	mkdir -p $(local_path)
	go fmt ./... && go build -o $(local_path)/terraform-provider-zfs

.PHONY: docker-build
docker-build: lint
	mkdir -p $(local_path)
	DOCKER_BUILDKIT=1 docker build --output $(local_path)/ .

.PHONY: lint
lint:
	docker run -it --rm -v $(pwd):/data docker.io/cytopia/golint .