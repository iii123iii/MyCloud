package com.mycloud.core.work

import android.content.ContentResolver
import android.net.Uri
import okhttp3.MediaType
import okhttp3.MediaType.Companion.toMediaTypeOrNull
import okhttp3.RequestBody
import okio.BufferedSink
import java.io.IOException

/**
 * Streams a `content://` URI as a multipart body, reporting bytes written so the
 * worker can publish upload progress. Avoids loading the whole file into memory.
 */
class UriRequestBody(
    private val resolver: ContentResolver,
    private val uri: Uri,
    private val contentType: String,
    private val contentLength: Long,
    private val onProgress: (sent: Long, total: Long) -> Unit,
) : RequestBody() {

    override fun contentType(): MediaType? = contentType.toMediaTypeOrNull()

    override fun contentLength(): Long = contentLength

    override fun writeTo(sink: BufferedSink) {
        val input = resolver.openInputStream(uri) ?: throw IOException("Cannot open $uri")
        input.use { stream ->
            val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
            var uploaded = 0L
            var read = stream.read(buffer)
            while (read != -1) {
                sink.write(buffer, 0, read)
                uploaded += read
                onProgress(uploaded, contentLength)
                read = stream.read(buffer)
            }
        }
    }

    private companion object {
        const val DEFAULT_BUFFER_SIZE = 8 * 1024
    }
}
