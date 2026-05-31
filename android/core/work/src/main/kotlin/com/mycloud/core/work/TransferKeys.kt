package com.mycloud.core.work

/** Shared WorkManager input/output data keys and tags for transfers. */
object TransferKeys {
    const val KEY_URI = "uri"
    const val KEY_NAME = "name"
    const val KEY_FOLDER_ID = "folder_id"
    const val KEY_FILE_ID = "file_id"
    /** Path to a temp file holding the archive selection — avoids WorkManager's
     *  ~10 KB Data limit for large selections (Data only carries this short path). */
    const val KEY_ARCHIVE_REQUEST = "archive_request_path"
    const val KEY_PROGRESS = "progress"
    const val KEY_OUTPUT_URI = "output_uri"

    const val TAG_TRANSFER = "transfer"
    const val TAG_UPLOAD = "transfer_upload"
    const val TAG_DOWNLOAD = "transfer_download"

    /** Display name carried as a tag so it's visible while QUEUED (before any progress). */
    const val NAME_TAG_PREFIX = "name:"
}
