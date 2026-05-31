package com.mycloud.data.config

import com.mycloud.core.datastore.settings.ServerConfig
import com.mycloud.core.network.config.ServerUrlSource
import javax.inject.Inject
import javax.inject.Singleton

@Singleton
class ServerUrlSourceImpl @Inject constructor(
    private val serverConfig: ServerConfig,
) : ServerUrlSource {
    override fun currentBaseUrl(): String = serverConfig.current()
}
