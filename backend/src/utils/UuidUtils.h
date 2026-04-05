#pragma once
#include <string>
#include <uuid/uuid.h>

namespace utils {

inline std::string generateUuid() {
    uuid_t raw;
    uuid_generate_random(raw);
    char buf[37];
    uuid_unparse_lower(raw, buf);
    return std::string(buf);
}

} // namespace utils
