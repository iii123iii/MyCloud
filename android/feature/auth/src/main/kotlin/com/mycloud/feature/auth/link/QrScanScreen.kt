@file:OptIn(androidx.compose.material3.ExperimentalMaterial3ExpressiveApi::class)

package com.mycloud.feature.auth.link

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.widthIn
import androidx.activity.compose.BackHandler
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ErrorOutline
import androidx.compose.material.icons.filled.QrCodeScanner
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning

/**
 * Opens Google Play Services' Code Scanner to scan the website QR sign-in code.
 * This keeps scanner models/native code out of our APK while preserving the
 * existing device-link flow once a raw QR payload is returned.
 */
@Composable
fun QrScanScreen(
    onBack: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: QrScanViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val context = androidx.compose.ui.platform.LocalContext.current
    val scanner = remember(context) {
        val options = GmsBarcodeScannerOptions.Builder()
            .setBarcodeFormats(Barcode.FORMAT_QR_CODE)
            .enableAutoZoom()
            .build()
        GmsBarcodeScanning.getClient(context, options)
    }
    var scanRequest by remember { mutableIntStateOf(0) }
    var scanInFlight by remember { mutableStateOf(false) }

    fun leaveScanner() {
        viewModel.cancelAndReset()
        onBack()
    }

    BackHandler(onBack = ::leaveScanner)

    LaunchedEffect(scanRequest, state.phase) {
        if (scanRequest == 0 || state.phase != QrScanUiState.Phase.SCANNING || scanInFlight) return@LaunchedEffect
        scanInFlight = true
        scanner.startScan()
            .addOnSuccessListener { barcode ->
                barcode.rawValue?.let(viewModel::onQrDetected)
                    ?: viewModel.onScannerUnavailable("The scanner didn't return a QR payload.")
            }
            .addOnCanceledListener {
                viewModel.onScanCanceled()
            }
            .addOnFailureListener {
                viewModel.onScannerUnavailable(
                    "QR scanning requires Google Play Services on this device. You can still sign in with your password.",
                )
            }
            .addOnCompleteListener {
                scanInFlight = false
            }
    }

    Surface(modifier = modifier.fillMaxSize()) {
        Box(Modifier.fillMaxSize()) {
            IconButton(
                onClick = ::leaveScanner,
                modifier = Modifier
                    .statusBarsPadding()
                    .padding(8.dp)
                    .align(Alignment.TopStart),
            ) {
                Icon(
                    imageVector = Icons.AutoMirrored.Filled.ArrowBack,
                    contentDescription = "Back to sign in",
                )
            }

            when (state.phase) {
                QrScanUiState.Phase.SCANNING -> ScanPrompt(
                    onScan = { scanRequest++ },
                    enabled = !scanInFlight,
                )
                QrScanUiState.Phase.LINKING -> LinkingContent()
                QrScanUiState.Phase.ERROR -> ErrorContent(
                    message = state.errorMessage ?: "Couldn't link this device.",
                    onRetry = {
                        viewModel.retry()
                        scanRequest++
                    },
                )
            }
        }
    }
}

@Composable
private fun ScanPrompt(onScan: () -> Unit, enabled: Boolean) {
    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(PaddingValues(24.dp)),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        Icon(
            imageVector = Icons.Filled.QrCodeScanner,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.primary,
            modifier = Modifier.size(48.dp),
        )
        Spacer(Modifier.height(16.dp))
        Text(
            text = "Scan sign-in QR code",
            style = MaterialTheme.typography.titleMedium,
            textAlign = TextAlign.Center,
        )
        Spacer(Modifier.height(8.dp))
        Text(
            text = "Point your camera at the QR code shown on the MyCloud website.",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
            modifier = Modifier.widthIn(max = 320.dp),
        )
        Spacer(Modifier.height(20.dp))
        Button(
            onClick = onScan,
            enabled = enabled,
            modifier = Modifier.fillMaxWidth().widthIn(max = 320.dp),
        ) {
            Text(if (enabled) "Open scanner" else "Scanner open")
        }
    }
}

@Composable
private fun LinkingContent() {
    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        LoadingIndicator()
        Spacer(Modifier.height(16.dp))
        Text(
            text = "Waiting for approval on your other device...",
            style = MaterialTheme.typography.bodyLarge,
            textAlign = TextAlign.Center,
            modifier = Modifier.widthIn(max = 300.dp),
        )
    }
}

@Composable
private fun ErrorContent(
    message: String,
    onRetry: () -> Unit,
) {
    Column(
        modifier = Modifier.fillMaxSize().padding(24.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        Icon(
            imageVector = Icons.Filled.ErrorOutline,
            contentDescription = null,
            tint = MaterialTheme.colorScheme.error,
            modifier = Modifier.size(48.dp),
        )
        Spacer(Modifier.height(16.dp))
        Text(
            text = message,
            style = MaterialTheme.typography.bodyLarge,
            textAlign = TextAlign.Center,
            modifier = Modifier.widthIn(max = 300.dp),
        )
        Spacer(Modifier.height(20.dp))
        Button(onClick = onRetry) {
            Text("Scan again")
        }
    }
}
