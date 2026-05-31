package com.mycloud.core.model

/** An item in the trash (GET /trash). */
data class TrashItem(
    val type: String, // "file" | "folder"
    val id: String,
    val name: String,
    val sizeBytes: Long,
    val mimeType: String?,
    val deletedAtMillis: Long,
) {
    val isFolder: Boolean get() = type == "folder"
}
