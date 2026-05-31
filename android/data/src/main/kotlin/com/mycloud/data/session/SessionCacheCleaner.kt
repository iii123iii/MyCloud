package com.mycloud.data.session

/**
 * Wipes all on-device cache that could leak one user's data to another: the Room
 * cache (files/folders/paging keys), the Coil image cache, and downloaded preview
 * files. Invoked on sign-out and when a different user signs in.
 *
 * The implementation lives in `:app` (it needs the Coil singleton); it is bound to
 * this interface so the `:data` auth layer can trigger a wipe without depending on
 * the image stack.
 */
interface SessionCacheCleaner {
    suspend fun clear()
}
