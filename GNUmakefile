default: testacc
version = 0.1.1

# Run acceptance tests
.PHONY: testacc
testacc:
	TF_ACC=1 go test ./... -v $(TESTARGS) -timeout 120m

.PHONY: build-local
build-local:
	mkdir -p ~/.terraform.d/plugins/local/MathiasPius/zfs/terraform-provider-zfs/$(version)/
	go fmt ./... && go build -o ~/.terraform.d/plugins/local/MathiasPius/zfs/terraform-provider-zfs/$(version)/terraform-provider-zfs