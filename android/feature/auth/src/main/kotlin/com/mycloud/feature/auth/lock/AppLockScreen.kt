package com.mycloud.feature.auth.lock

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle

@Composable
fun AppLockScreen(
    modifier: Modifier = Modifier,
    viewModel: AppLockViewModel = hiltViewModel(),
) {
    val context = LocalContext.current
    val activity = remember(context) { context.findFragmentActivity() }
    val errorMessage by viewModel.errorMessage.collectAsStateWithLifecycle()

    fun prompt() {
        val host = activity ?: run { viewModel.unlock(); return }
        promptBiometricUnlock(
            host,
            onSuccess = viewModel::unlock,
            onError = viewModel::onError,
            onUnavailable = viewModel::onError,
        )
    }

    // Consume system back so predictive-back can't peel the lock away and expose content.
    BackHandler(enabled = true) {}

    // Auto-present the biometric prompt when the lock screen appears.
    LaunchedEffect(Unit) { prompt() }

    Surface(modifier = modifier.fillMaxSize()) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .statusBarsPadding()
                .verticalScroll(rememberScrollState())
                .imePadding()
                .padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Icon(
                imageVector = Icons.Filled.Lock,
                contentDescription = null,
                tint = MaterialTheme.colorScheme.primary,
                modifier = Modifier.size(48.dp),
            )
            Spacer(Modifier.height(16.dp))
            Text("MyCloud is locked", style = MaterialTheme.typography.titleMedium)

            if (errorMessage != null) {
                Spacer(Modifier.height(12.dp))
                Text(
                    text = errorMessage!!,
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodySmall,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                )
            }

            Spacer(Modifier.height(16.dp))
            Button(onClick = { prompt() }) {
                Text("Unlock")
            }
            Spacer(Modifier.height(8.dp))
            TextButton(onClick = viewModel::signOut) {
                Text("Sign out")
            }
        }
    }
}
