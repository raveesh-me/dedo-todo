include Makefile.api
include Makefile.kind

# commerce-auth's and commerce-obs's options ship no Go code: generate it
# from the commits buf.lock pins before the service's own code, which
# imports it. A prerequisite, so Makefile.api's generate stays dedo's.
.PHONY: generate-options
generate-options:
	buf generate --template buf.gen.options.yaml

generate: generate-options
