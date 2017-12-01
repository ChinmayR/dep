# Returns a non-empty string if any of the items in $1 is not inside
# idl/code.uber.internal
define is-external-idl
$(if $(patsubst idl/code.uber.internal/%,,$1),Y,)
endef

# If all IDLs are contained in idl/code.uber.internal, use that as the Thrift
# root for friendlier package paths. Otherwise use idl/.
THRIFTRW_SRCS_ROOT = $(strip \
	$(if $(call is-external-idl,$(THRIFTRW_SRCS)), \
		idl/, \
		idl/code.uber.internal/))

# Names of all ThriftRW plugins specified by the user.
THRIFTRW_PLUGIN_NAMES = $(patsubst thriftrw-plugin-%, %, $(notdir $(THRIFTRW_PLUGINS)))
THRIFTRW_IMPORT_PATH := go.uber.org/thriftrw

thriftsync: $(GLIDE_EXEC) $(GLIDE) ## Syncs thrift handler files with the provided idl. more info: http://t.uber.com/thriftsync
	$(ECHO_V)$(GLIDE_EXEC) --glide $(GLIDE) --bin $(_GOPATH)/bin \
	--exe $(THRIFTRW_IMPORT_PATH) --exe go.uber.org/fx/modules/yarpc/thriftrw-plugin-thriftsync \
	$(foreach source, $(SERVICE_IDLS), \
	thriftrw --pkg-prefix $(PROJECT_ROOT)/$(THRIFT_GENDIR)/go/ --out $(THRIFT_GENDIR)/go --thrift-root $(THRIFTRW_SRCS_ROOT) --plugin="thriftsync $(THRIFTSYNC_ARGS)" $(source))

