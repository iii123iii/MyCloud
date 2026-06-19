package com.mycloud.core.common.util

/** Coarse file kind used to pick an icon and a preview surface. */
enum class FileKind { IMAGE, VIDEO, AUDIO, PDF, TEXT, ARCHIVE, OTHER }

object MimeType {

    fun kindOf(mimeType: String?): FileKind {
        val mime = mimeType?.lowercase().orEmpty()
        return when {
            mime.startsWith("image/") -> FileKind.IMAGE
            mime.startsWith("video/") -> FileKind.VIDEO
            mime.startsWith("audio/") -> FileKind.AUDIO
            mime == "application/pdf" -> FileKind.PDF
            mime.startsWith("text/") ||
                mime == "application/json" ||
                mime == "application/javascript" ||
                mime == "application/x-javascript" ||
                mime == "application/xml" -> FileKind.TEXT
            mime in archiveMimes -> FileKind.ARCHIVE
            else -> FileKind.OTHER
        }
    }

    /** Whether a file of this type can be previewed inline (vs. download-only). */
    fun isPreviewable(mimeType: String?): Boolean =
        when (kindOf(mimeType)) {
            FileKind.IMAGE, FileKind.VIDEO, FileKind.AUDIO, FileKind.PDF, FileKind.TEXT -> true
            else -> false
        }

    /**
     * Uppercase extension badge from a filename (max 4 chars), like the web tiles.
     * Returns "" when there's no real extension — handles blank/null names and
     * dot-files (e.g. ".gitignore" has no extension, not "GITIG").
     */
    fun extensionBadge(name: String?): String {
        val trimmed = name?.trim().orEmpty()
        if (trimmed.isEmpty()) return ""
        val dot = trimmed.lastIndexOf('.')
        // No dot, or a leading-dot dot-file (".bashrc"), or a trailing dot → no ext.
        if (dot <= 0 || dot == trimmed.lastIndex) return ""
        return trimmed.substring(dot + 1).take(4).uppercase()
    }

    private val archiveMimes = setOf(
        "application/zip",
        "application/x-zip-compressed",
        "application/x-tar",
        "application/gzip",
        "application/x-7z-compressed",
        "application/x-rar-compressed",
        "application/vnd.rar",
    )
}
