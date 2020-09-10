IMAGE    ?= lmapii/txwifi
NAME     ?= txwifi
VERSION  ?= 1.0.4

all: build push

dev: dev_build dev_run

build:
	docker build -t $(IMAGE):latest .
	docker build -t $(IMAGE):arm32v6-$(VERSION) .

push:
	docker push $(IMAGE):latest
	docker push $(IMAGE):arm32v6-$(VERSION)

dev_build:
	docker build -t $(IMAGE) ./dev/

dev_run:
	sudo docker run --rm -it --privileged --network=host \
                   -v $(CURDIR):/go/src/github.com/lmapii/txwifi \
                   -w /go/src/github.com/lmapii/txwifi \
                   --name=$(NAME) $(IMAGE):latest


