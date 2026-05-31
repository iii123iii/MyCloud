package com.mycloud.core.work

import java.io.File
import java.util.UUID

/**
 * Persists an archive selection (file + folder ids) to a small text file in
 * `filesDir`, so [ArchiveDownloadWorker] can be handed the selection by path
 * rather than through WorkManager's size-limited (~10 KB) input `Data`. This
 * keeps arbitrarily large selections working.
 *
 * Format: line 0 = number of file ids, then the file ids, then the folder ids.
 * Ids are server UUIDs, which never contain newlines, so a line-based format is
 * unambiguous and needs no serialization dependency.
 */
internal object ArchiveRequestStore {

    fun write(filesDir: File, fileIds: List<String>, folderIds: List<String>): File {
        val dir = File(filesDir, "archive_requests").apply { mkdirs() }
        val out = File(dir, "${UUID.randomUUID()}.txt")
        out.writeText(
            buildString {
                append(fileIds.size).append('\n')
                fileIds.forEach { append(it).append('\n') }
                folderIds.forEach { append(it).append('\n') }
            },
        )
        return out
    }

    /** Returns (fileIds, folderIds), or null if the file is missing/malformed. */
    fun read(file: File): Pair<List<String>, List<String>>? = runCatching {
        val lines = file.readLines()
        val fileCount = lines.first().toInt()
        val fileIds = lines.subList(1, 1 + fileCount).toList()
        val folderIds = lines.subList(1 + fileCount, lines.size).toList()
        fileIds to folderIds
    }.getOrNull()
}
