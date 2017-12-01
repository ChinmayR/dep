# Verbosity rules
#
# Setting V=1 during builds removes the output inhibitors for any command
# prefixed with $(ECHO_V).

V ?= 0
ifeq ($(V),0)
  ECHO_V = @
else
  TEST_VERBOSITY_FLAG = -v
endif

LABEL_STYLE = "$(COLOR_BOLD)$(COLOR_COMMAND)"

GOVERSION_LABEL = $(shell $(GO) version | cut -d" " -f3-4 | sed s/go// )
THRIFTC_LABEL = "$(LABEL_STYLE)  THRIFTC                  $(COLOR_RESET)"
THRIFTRW_LABEL = "$(LABEL_STYLE)  THRIFTRW   $(COLOR_RESET)"
PROTOC_LABEL = "$(LABEL_STYLE)  PROTOC   $(COLOR_RESET)"
GOBUILD_LABEL = "$(LABEL_STYLE)  GO BUILD ($(GOVERSION_LABEL))   $(COLOR_RESET)"
