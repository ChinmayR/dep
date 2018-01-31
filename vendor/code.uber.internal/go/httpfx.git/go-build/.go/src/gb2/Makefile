include ../../../include.mk

.PHONY: update-vendor
update-vendor: $(GLIDE)
	@# ensure glide exists before blowing away vendor
	$(GLIDE) --version
	cp $(GLIDE) ./glide # we are deleting the glide deps below; root glide is just a script, get the built glide
	./glide --debug up
	rm -rf vendor
	./glide --debug install
	rm -f ./glide
	find vendor \( -not -iname '*.go' -or -iname '*_test.go' \) -and -type f -delete
	find vendor -iname testdata -or -iname .git -or -iname fixtures | xargs rm -rf
	rm -rf vendor/github.com/Masterminds/glide/vendor/github.com/Masterminds/vcs/

