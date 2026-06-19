package com.mycloud.android.ui

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycloud.android.RootViewModel
import com.mycloud.core.model.AuthState
import com.mycloud.feature.auth.lock.AppLockScreen
import com.mycloud.feature.auth.link.QrScanScreen
import com.mycloud.feature.auth.login.LoginScreen
import com.mycloud.feature.auth.password.ChangePasswordScreen

/**
 * Root navigation gate. Renders the right top-level surface for the current
 * [AuthState]; signals [onReady] once state resolves so the splash can dismiss.
 * The Authenticated branch hosts the [AppShell].
 */
@Composable
fun RootContent(
    onReady: () -> Unit,
    onEnterMainShell: () -> Unit = {},
    deepLink: android.net.Uri? = null,
    onDeepLinkHandled: () -> Unit = {},
    viewModel: RootViewModel = hiltViewModel(),
) {
    val gate by viewModel.gate.collectAsStateWithLifecycle()

    // Signal ready exactly once, on the first non-LOADING state, so the splash
    // dismisses once and later auth transitions don't re-fire it.
    val currentOnReady by rememberUpdatedState(onReady)
    LaunchedEffect(gate != AuthState.LOADING) {
        if (gate != AuthState.LOADING) currentOnReady()
    }

    AnimatedContent(
        targetState = gate,
        transitionSpec = { fadeIn() togetherWith fadeOut() },
        label = "authGate",
    ) { state ->
        when (state) {
            AuthState.LOADING ->
                Box(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.background))
            AuthState.LOGGED_OUT -> {
                // Local sub-navigation between the password form and the QR
                // scanner. On a successful scan the repository flips AuthState,
                // so this whole branch is swapped out by AnimatedContent.
                var showScanner by rememberSaveable { mutableStateOf(false) }
                if (showScanner) {
                    QrScanScreen(onBack = { showScanner = false })
                } else {
                    LoginScreen(onScanQr = { showScanner = true })
                }
            }
            AuthState.NEEDS_PASSWORD_CHANGE -> ChangePasswordScreen()
            AuthState.LOCKED -> AppLockScreen()
            AuthState.AUTHENTICATED -> AppShell(
                onSignOut = viewModel::logout,
                onEnterMainShell = onEnterMainShell,
                deepLink = deepLink,
                onDeepLinkHandled = onDeepLinkHandled,
            )
        }
    }
}
