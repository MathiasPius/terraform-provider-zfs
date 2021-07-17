FROM golang:1.16-alpine AS build
RUN apk add curl
WORKDIR /src
COPY go.mod .
COPY go.sum .
RUN go mod download
RUN curl https://github.com/dominikh/go-tools/releases/download/2021.1/staticcheck_linux_arm64.tar.gz | tar -xz -C /usr/local/bin staticcheck/staticcheck && chmod +x /usr/local/bin/staticcheck
COPY . .
RUN go fmt ./...
RUN go vet
RUN go build -o /out/terraform-provider-zfs .
FROM scratch AS bin
COPY --from=build /out/terraform-provider-zfs /
