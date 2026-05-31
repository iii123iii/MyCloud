package com.mycloud.core.model

/** Access level a caller has on a shared file/folder. */
enum class Permission { READ, WRITE, OWNER }

/** Columns the file listing can be sorted by (maps to the `sort` query param). */
enum class SortKey(val wire: String) {
    NAME("name"),
    SIZE("size_bytes"),
    CREATED("created_at"),
    UPDATED("updated_at"),
}

enum class SortOrder(val wire: String) { ASC("ASC"), DESC("DESC") }

/** How the browser renders a folder's contents. */
enum class ViewMode { GRID, LIST }

/** The authenticated principal's role. */
enum class UserRole { USER, ADMIN }
