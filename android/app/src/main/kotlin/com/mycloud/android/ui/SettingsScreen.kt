@file:OptIn(androidx.compose.material3.ExperimentalMaterial3ExpressiveApi::class)

package com.mycloud.android.ui

import android.app.KeyguardManager
import android.text.format.DateUtils
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.LoadingIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.core.content.getSystemService
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycloud.core.common.util.ByteFormatter
import com.mycloud.core.datastore.settings.ThemeMode
import com.mycloud.core.model.ActivityEntry
import com.mycloud.core.model.PersonalAccessToken
import com.mycloud.core.model.Session

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    onSignOut: () -> Unit,
    modifier: Modifier = Modifier,
    viewModel: SettingsViewModel = hiltViewModel(),
) {
    val user by viewModel.user.collectAsStateWithLifecycle()
    val state by viewModel.state.collectAsStateWithLifecycle()
    val themeMode by viewModel.themeMode.collectAsStateWithLifecycle()
    val appLockEnabled by viewModel.appLockEnabled.collectAsStateWithLifecycle()
    val context = LocalContext.current
    val deviceSecure = remember { context.getSystemService<KeyguardManager>()?.isDeviceSecure == true }
    var showCreateToken by remember { mutableStateOf(false) }
    var showChangePassword by remember { mutableStateOf(false) }
    var showDeleteAccount by remember { mutableStateOf(false) }
    var showSignOutConfirm by remember { mutableStateOf(false) }

    Column(
        modifier = modifier
            .fillMaxSize()
            .statusBarsPadding()
            .verticalScroll(rememberScrollState())
            .padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text("Settings", style = MaterialTheme.typography.headlineSmall)

        (state.actionError ?: state.accountError)?.let { message ->
            Text(
                text = message,
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier
                    .fillMaxWidth()
                    .semantics { liveRegion = LiveRegionMode.Polite },
            )
        }

        SectionTitle("Account")
        user?.let { current ->
            Text(current.username, style = MaterialTheme.typography.titleMedium)
            Text(
                current.email,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        } ?: Row(verticalAlignment = Alignment.CenterVertically) {
            LoadingIndicator(Modifier.size(18.dp))
            Spacer(Modifier.size(8.dp))
            EmptyHint("Loading account…")
        }

        HorizontalDivider()
        SectionTitle("Appearance")
        val themeOptions = listOf(ThemeMode.SYSTEM to "System", ThemeMode.LIGHT to "Light", ThemeMode.DARK to "Dark")
        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
            themeOptions.forEachIndexed { index, (mode, label) ->
                SegmentedButton(
                    selected = themeMode == mode,
                    onClick = { viewModel.setTheme(mode) },
                    shape = SegmentedButtonDefaults.itemShape(index, themeOptions.size),
                ) { Text(label) }
            }
        }

        HorizontalDivider()
        SectionTitle("Storage")
        val stats = state.stats
        when {
            stats != null -> {
                LinearProgressIndicator(
                    progress = { ByteFormatter.usedFraction(stats.usedBytes, stats.quotaBytes) },
                    modifier = Modifier.fillMaxWidth(),
                )
                Text(
                    "${ByteFormatter.format(stats.usedBytes)} of ${ByteFormatter.format(stats.quotaBytes)} used",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            state.statsLoading -> Row(verticalAlignment = Alignment.CenterVertically) {
                LoadingIndicator(Modifier.size(18.dp))
                Spacer(Modifier.size(8.dp))
                EmptyHint("Loading storage usage…")
            }
            else -> {
                EmptyHint(state.statsError ?: "Couldn't load storage usage.")
                TextButton(onClick = viewModel::loadStats) { Text("Retry") }
            }
        }

        HorizontalDivider()
        SectionTitle("Sessions")
        if (state.sessions.isEmpty()) {
            EmptyHint("No active sessions.")
        } else {
            state.sessions.forEach { session ->
                SessionRow(session, onRevoke = { viewModel.revokeSession(session.jti) })
            }
        }

        HorizontalDivider()
        SectionTitle("Recent activity")
        if (state.activity.isEmpty()) {
            EmptyHint("No recent activity.")
        } else {
            state.activity.take(20).forEach { entry -> ActivityRow(entry) }
        }

        HorizontalDivider()
        Row(verticalAlignment = Alignment.CenterVertically) {
            SectionTitle("Access tokens", modifier = Modifier.weight(1f))
            TextButton(onClick = { showCreateToken = true }) { Text("Create") }
        }
        if (state.tokens.isEmpty()) {
            EmptyHint("No personal access tokens.")
        } else {
            state.tokens.forEach { token ->
                TokenRow(token, onRevoke = { viewModel.revokeToken(token.id) })
            }
        }

        HorizontalDivider()
        SectionTitle("Security")
        Row(verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text("App lock", style = MaterialTheme.typography.bodyLarge)
                Text(
                    if (deviceSecure) "Require biometrics or device PIN to open MyCloud"
                    else "Set up a device screen lock to enable",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Switch(
                checked = appLockEnabled && deviceSecure,
                onCheckedChange = { viewModel.setAppLockEnabled(it) },
                enabled = deviceSecure,
            )
        }
        Spacer(Modifier.height(8.dp))
        OutlinedButton(
            onClick = { showChangePassword = true },
            modifier = Modifier.fillMaxWidth(),
        ) { Text("Change password") }

        Spacer(Modifier.height(8.dp))
        OutlinedButton(onClick = { showSignOutConfirm = true }, modifier = Modifier.fillMaxWidth()) { Text("Sign out") }

        Spacer(Modifier.height(8.dp))
        SectionTitle("Danger zone")
        Button(
            onClick = { showDeleteAccount = true },
            modifier = Modifier.fillMaxWidth(),
            colors = ButtonDefaults.buttonColors(
                containerColor = MaterialTheme.colorScheme.error,
                contentColor = MaterialTheme.colorScheme.onError,
            ),
        ) { Text("Delete account") }
    }

    if (showChangePassword) {
        ChangePasswordDialog(
            busy = state.passwordBusy,
            error = state.passwordError,
            done = state.passwordChanged,
            onConfirm = { old, new -> viewModel.changePassword(old, new) },
            onDismiss = {
                showChangePassword = false
                viewModel.ackPasswordResult()
            },
        )
    }

    if (showDeleteAccount) {
        DeleteAccountDialog(
            busy = state.deleteBusy,
            error = state.deleteError,
            onConfirm = { password -> viewModel.deleteAccount(password) },
            onDismiss = {
                showDeleteAccount = false
                viewModel.ackDeleteError()
            },
        )
    }

    if (showSignOutConfirm) {
        AlertDialog(
            onDismissRequest = { showSignOutConfirm = false },
            title = { Text("Sign out?") },
            text = { Text("You'll need to sign in again to access your files on this device.") },
            confirmButton = {
                TextButton(onClick = {
                    showSignOutConfirm = false
                    onSignOut()
                }) { Text("Sign out") }
            },
            dismissButton = { TextButton(onClick = { showSignOutConfirm = false }) { Text("Cancel") } },
        )
    }

    if (showCreateToken) {
        CreateTokenDialog(
            error = state.createTokenError,
            // The secret being issued closes the create dialog and opens SecretDialog.
            secretIssued = state.createdTokenSecret != null,
            onConfirm = { viewModel.createToken(it) },
            onDismiss = {
                showCreateToken = false
                viewModel.clearCreateTokenError()
            },
        )
    }

    state.createdTokenSecret?.let { secret ->
        // No outside-dismiss: the one-time secret is only retrievable here.
        SecretDialog(
            secret = secret,
            onDone = {
                viewModel.clearCreatedToken()
                showCreateToken = false
            },
        )
    }
}

@Composable
private fun SectionTitle(text: String, modifier: Modifier = Modifier) {
    Text(text, style = MaterialTheme.typography.titleSmall, modifier = modifier)
}

@Composable
private fun EmptyHint(text: String) {
    Text(text, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
}

@Composable
private fun SessionRow(session: Session, onRevoke: () -> Unit) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(
                (session.deviceLabel ?: "Unknown device") + if (session.isCurrent) " (this device)" else "",
                style = MaterialTheme.typography.bodyMedium,
            )
            session.ipAddress?.let {
                Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        if (!session.isCurrent) {
            val label = session.deviceLabel ?: "this device"
            TextButton(
                onClick = onRevoke,
                modifier = Modifier.semantics { contentDescription = "Revoke session for $label" },
            ) { Text("Revoke") }
        }
    }
}

@Composable
private fun ActivityRow(entry: ActivityEntry) {
    Column {
        Text(
            buildString {
                append(entry.action)
                entry.resourceType?.let { append(" • $it") }
            },
            style = MaterialTheme.typography.bodySmall,
        )
        Text(
            DateUtils.getRelativeTimeSpanString(
                entry.createdAtMillis,
                System.currentTimeMillis(),
                DateUtils.MINUTE_IN_MILLIS,
            ).toString(),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun TokenRow(token: PersonalAccessToken, onRevoke: () -> Unit) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(token.name, style = MaterialTheme.typography.bodyMedium)
            val suffix = listOfNotNull(token.tokenPrefix, token.lastFour).joinToString("…")
            if (suffix.isNotBlank()) {
                Text(suffix, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        IconButton(onClick = onRevoke) {
            Icon(Icons.Filled.Delete, contentDescription = "Revoke token ${token.name}")
        }
    }
}

@Composable
private fun CreateTokenDialog(
    error: String?,
    secretIssued: Boolean,
    onConfirm: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    var name by remember { mutableStateOf("") }
    var submitting by remember { mutableStateOf(false) }
    // The secret arriving (or an error) ends the in-flight state.
    LaunchedEffect(secretIssued, error) {
        if (secretIssued || error != null) submitting = false
    }
    AlertDialog(
        onDismissRequest = { if (!submitting) onDismiss() },
        title = { Text("New access token") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    singleLine = true,
                    enabled = !submitting,
                    label = { Text("Token name") },
                )
                error?.let {
                    Text(
                        it,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = { submitting = true; onConfirm(name) },
                enabled = name.isNotBlank() && !submitting,
            ) {
                if (submitting) LoadingIndicator(Modifier.size(18.dp)) else Text("Create")
            }
        },
        dismissButton = { TextButton(onClick = onDismiss, enabled = !submitting) { Text("Cancel") } },
    )
}

@Composable
private fun ChangePasswordDialog(
    busy: Boolean,
    error: String?,
    done: Boolean,
    onConfirm: (old: String, new: String) -> Unit,
    onDismiss: () -> Unit,
) {
    var oldPassword by remember { mutableStateOf("") }
    var newPassword by remember { mutableStateOf("") }
    var confirmPassword by remember { mutableStateOf("") }

    // Close automatically once the change succeeds.
    LaunchedEffect(done) { if (done) onDismiss() }

    val mismatch = confirmPassword.isNotEmpty() && newPassword != confirmPassword
    val canSubmit = !busy &&
        oldPassword.isNotBlank() &&
        newPassword.length >= 8 &&
        newPassword == confirmPassword

    AlertDialog(
        onDismissRequest = { if (!busy) onDismiss() },
        title = { Text("Change password") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(
                    value = oldPassword,
                    onValueChange = { oldPassword = it },
                    label = { Text("Current password") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = newPassword,
                    onValueChange = { newPassword = it },
                    label = { Text("New password") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                )
                OutlinedTextField(
                    value = confirmPassword,
                    onValueChange = { confirmPassword = it },
                    label = { Text("Confirm new password") },
                    singleLine = true,
                    isError = mismatch,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                )
                if (mismatch) {
                    Text("Passwords don't match", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
                }
                error?.let {
                    Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { onConfirm(oldPassword, newPassword) }, enabled = canSubmit) {
                if (busy) LoadingIndicator(Modifier.size(18.dp)) else Text("Update")
            }
        },
        dismissButton = { TextButton(onClick = onDismiss, enabled = !busy) { Text("Cancel") } },
    )
}

@Composable
private fun DeleteAccountDialog(
    busy: Boolean,
    error: String?,
    onConfirm: (password: String) -> Unit,
    onDismiss: () -> Unit,
) {
    var password by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = { if (!busy) onDismiss() },
        title = { Text("Delete account") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    "This permanently deletes your account and all of your files. This cannot be undone.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text("Confirm with your password") },
                    singleLine = true,
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth(),
                )
                error?.let {
                    Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onConfirm(password) },
                enabled = !busy && password.isNotBlank(),
                colors = ButtonDefaults.textButtonColors(contentColor = MaterialTheme.colorScheme.error),
            ) {
                if (busy) LoadingIndicator(Modifier.size(18.dp)) else Text("Delete")
            }
        },
        dismissButton = { TextButton(onClick = onDismiss, enabled = !busy) { Text("Cancel") } },
    )
}

@Composable
private fun SecretDialog(secret: String, onDone: () -> Unit) {
    val clipboard = LocalClipboardManager.current
    AlertDialog(
        // Only dismiss via explicit Copy/Done — tapping outside must not lose the one-time secret.
        onDismissRequest = {},
        title = { Text("Token created") },
        text = {
            Column {
                Text(
                    "Copy this token now — it won't be shown again.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(8.dp))
                Text(secret, style = MaterialTheme.typography.bodyMedium)
            }
        },
        confirmButton = {
            Button(onClick = { clipboard.setText(AnnotatedString(secret)) }) { Text("Copy") }
        },
        dismissButton = { TextButton(onClick = onDone) { Text("Done") } },
    )
}
