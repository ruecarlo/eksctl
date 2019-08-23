build_image_input = build_image.mk build_image.dkr install-build-deps.sh go.mod go.sum

build_image_tag := $(shell git ls-tree --full-tree @ -- build_image.sum | awk '{ print $$3 }')

build_image_name := weaveworks/eksctl-build:$(build_image_tag)

.PHONY: update-image-tag
update-image-tag:
	git ls-tree --full-tree @ -- $(build_image_input) > build_image.sum
	git commit --quiet build_image.sum --message 'Update build_image.sum'

.PHONY: image
build-image:
	-docker pull $(build_image_name)
	tar c $(build_image_input) | docker build --cache-from="$(build_image_name)" --tag="$(build_image_name)" --file=build_image.dkr -