package com.mycloud.core.network.api

import com.mycloud.core.network.dto.DownloadArchiveRequestDto
import com.mycloud.core.network.dto.FileDto
import com.mycloud.core.network.envelope.Envelope
import okhttp3.MultipartBody
import okhttp3.RequestBody
import okhttp3.ResponseBody
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.Multipart
import retrofit2.http.POST
import retrofit2.http.Part
import retrofit2.http.Path
import retrofit2.http.Streaming

/** Binary transfers. Upload is multipart (the worker supplies a progress-aware
 *  file part); download is streamed so large files aren't buffered in memory. */
interface TransferApi {

    // IMPORTANT: folder_id must precede the file part — the backend streams the
    // multipart body and stops reading once it reaches the file, so any field
    // after it (like folder_id) is ignored and the upload lands in the root.
    @Multipart
    @POST("api/v2/files:upload")
    suspend fun upload(
        @Part("folder_id") folderId: RequestBody?,
        @Part file: MultipartBody.Part,
    ): Response<Envelope<FileDto>>

    @Streaming
    @GET("api/v2/files/{id}:download")
    suspend fun download(@Path("id") id: String): Response<ResponseBody>

    /** Streams a server-built zip of the requested files + folder subtrees. */
    @Streaming
    @POST("api/v2/files:download-archive")
    suspend fun downloadArchive(@Body body: DownloadArchiveRequestDto): Response<ResponseBody>
}
