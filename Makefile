IMAGE ?= ghcr.io/moosilauke18/printodo-api

all:
	go build && ./printodo-api

docker: docker-build docker-push

docker-build:
	docker build -t $(IMAGE) .

docker-push:
	docker push $(IMAGE)
