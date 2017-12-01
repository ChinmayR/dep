# This is only an example Thrift interface.
# You should rewrite this file ;)
#
# This file will be pushed to rt/idl-registry everytime you
# land to master.

namespace java com.uber.engsec.pullo

exception UserNotFoundError {
    1: required string email,
}

exception Unauthorized {
    1: required string message,
}

service Pullo {

    bool isMemberOf(
        1: required string email,
        2: required string group,
    ) throws (
        1: UserNotFoundError notFound,
    )

    list<string> getUserGroups(
        1: required string email,
    ) throws (
        1: UserNotFoundError notFound,
    )
}
