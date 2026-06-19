package com.mycloud.core.network.api

import com.mycloud.core.network.dto.DeviceLinkClaimRequestDto
import com.mycloud.core.network.dto.DeviceLinkPollDto
import com.mycloud.core.network.dto.DeviceLinkPollRequestDto
import com.mycloud.core.network.dto.DeviceLinkStateDto
import com.mycloud.core.network.envelope.Envelope
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.POST

/**
 * Unauthenticated QR device-linking endpoints (built on the bare client — the
 * phone holds no token yet). Authority comes from the verifier carried in the
 * scanned QR plus the browser's approval.
 */
interface DeviceLinkApi {

    /** Claim a scanned code; submits this device's identity for the approval card. */
    @POST("api/v2/device-link/claim")
    suspend fun claim(@Body body: DeviceLinkClaimRequestDto): Response<Envelope<DeviceLinkStateDto>>

    /** Poll for the outcome; returns tokens once the browser approves. */
    @POST("api/v2/device-link/poll")
    suspend fun poll(@Body body: DeviceLinkPollRequestDto): Response<Envelope<DeviceLinkPollDto>>
}
