package com.mycloud.core.network.config

/**
 * Supplies the currently-configured backend base URL at request time. Implemented
 * in the data/datastore layer and bound into Hilt there. Lets the user change the
 * server address without rebuilding Retrofit (see HostSelectionInterceptor).
 */
interface ServerUrlSource {
    fun currentBaseUrl(): String
}
