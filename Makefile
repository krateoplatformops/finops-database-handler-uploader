ARCH?=amd64
REPO?=#your repository here 
VERSION?=0.1

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -o ./bin/database-handler-uploader main.go

container:
	docker build -t $(REPO)finops-database-handler-uploader:$(VERSION) .
	docker push $(REPO)finops-database-handler-uploader:$(VERSION)

container-multi:
	docker buildx build --tag $(REPO)finops-database-handler-uploader:$(VERSION) --push --platform linux/amd64,linux/arm64 .
