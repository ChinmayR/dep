ENV = env
CRAM = $(ENV)/bin/cram

.PHONY: build
build:
	go build

.PHONY: test
test: build $(CRAM)
	$(CRAM) testdata

$(CRAM): $(ENV)
	. $(ENV)/bin/activate && pip install cram

$(ENV):
	virtualenv $(ENV)
