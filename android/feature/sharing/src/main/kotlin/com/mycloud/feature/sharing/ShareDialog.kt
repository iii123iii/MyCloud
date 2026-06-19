package com.mycloud.feature.sharing

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Visibility
import androidx.compose.material.icons.filled.VisibilityOff
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.platform.LocalFocusManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycloud.core.model.Permission
import java.text.SimpleDateFormat
import java.time.Instant
import java.time.temporal.ChronoUnit
import java.util.Date
import java.util.Locale

/** Create-a-public-link dialog. On success it shows the link with a copy action. */
@Composable
fun ShareDialog(
    fileId: String?,
    folderId: String?,
    title: String,
    onDismiss: () -> Unit,
    viewModel: ShareViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val clipboard = LocalClipboardManager.current
    val focusManager = LocalFocusManager.current
    val created = state.createdLink

    // Don't allow dismissing mid-request — losing the dialog while a share/grant is
    // in flight would orphan the result.
    val inFlight = state.isCreating || state.isGranting
    val safeDismiss = { if (!inFlight) onDismiss() }

    // Clear any state left over from a previously-shared item (the VM is reused).
    LaunchedEffect(fileId, folderId) { viewModel.reset() }

    // Drop the keyboard once a collaborator was added, and auto-clear the confirmation.
    LaunchedEffect(state.grantMessage) {
        if (state.grantMessage != null) {
            focusManager.clearFocus()
            kotlinx.coroutines.delay(3_000)
            viewModel.clearGrantMessage()
        }
    }

    // Transient "Link copied" acknowledgement.
    var copiedAt by remember { mutableStateOf(0L) }
    var showCopied by remember { mutableStateOf(false) }
    LaunchedEffect(copiedAt) {
        if (copiedAt > 0L) {
            showCopied = true
            kotlinx.coroutines.delay(2_000)
            showCopied = false
        }
    }

    var passwordVisible by remember { mutableStateOf(false) }

    AlertDialog(
        onDismissRequest = safeDismiss,
        title = { Text(if (created == null) "Share “$title”" else "Link ready") },
        text = {
            if (created != null) {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(created, style = MaterialTheme.typography.bodyMedium)
                    if (showCopied) {
                        Text(
                            "Link copied",
                            color = MaterialTheme.colorScheme.primary,
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                }
            } else {
                Column(
                    modifier = Modifier.verticalScroll(rememberScrollState()),
                    verticalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    Text("Share with a person", style = MaterialTheme.typography.titleSmall)
                    OutlinedTextField(
                        value = state.grantee,
                        onValueChange = viewModel::setGrantee,
                        label = { Text("Username or email") },
                        singleLine = true,
                        isError = state.grantError != null,
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        PermissionSelector(
                            selected = state.grantPermission,
                            onSelect = viewModel::setGrantPermission,
                            readLabel = "Viewer",
                            writeLabel = "Editor",
                            modifier = Modifier.weight(1f),
                        )
                        TextButton(
                            onClick = { viewModel.addPerson(fileId, folderId) },
                            enabled = state.grantee.isNotBlank() && !state.isGranting,
                        ) { Text(if (state.isGranting) "Adding…" else "Add") }
                    }
                    state.grantMessage?.let {
                        Text(it, color = MaterialTheme.colorScheme.primary, style = MaterialTheme.typography.bodySmall)
                    }
                    state.grantError?.let {
                        Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
                    }

                    HorizontalDivider()
                    Text("Create a public link", style = MaterialTheme.typography.titleSmall)
                    PermissionSelector(
                        selected = state.permission,
                        onSelect = viewModel::setPermission,
                        readLabel = "View",
                        writeLabel = "Edit",
                        modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(
                        value = state.password,
                        onValueChange = viewModel::setPassword,
                        label = { Text("Password (optional)") },
                        singleLine = true,
                        visualTransformation = if (passwordVisible) {
                            VisualTransformation.None
                        } else {
                            PasswordVisualTransformation()
                        },
                        trailingIcon = {
                            IconButton(onClick = { passwordVisible = !passwordVisible }) {
                                Icon(
                                    if (passwordVisible) Icons.Filled.VisibilityOff else Icons.Filled.Visibility,
                                    contentDescription = if (passwordVisible) "Hide password" else "Show password",
                                )
                            }
                        },
                        modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(
                        value = state.expiresInDays,
                        onValueChange = viewModel::setExpiresInDays,
                        label = { Text("Expires in days (optional)") },
                        supportingText = {
                            resolvedExpiry(state.expiresInDays)?.let { Text("Expires $it") }
                        },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(
                        value = state.downloadLimit,
                        onValueChange = viewModel::setDownloadLimit,
                        label = { Text("Download limit (optional)") },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text("One-time view", modifier = Modifier.weight(1f))
                        Switch(checked = state.singleView, onCheckedChange = viewModel::setSingleView)
                    }
                    if (state.error != null) {
                        Text(
                            state.error!!,
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodySmall,
                        )
                    }
                }
            }
        },
        confirmButton = {
            if (created != null) {
                TextButton(onClick = {
                    clipboard.setText(AnnotatedString(created))
                    copiedAt = System.currentTimeMillis()
                }) { Text("Copy link") }
            } else {
                TextButton(
                    onClick = { viewModel.create(fileId, folderId) },
                    enabled = !state.isCreating,
                ) {
                    Text(if (state.isCreating) "Creating…" else "Create link")
                }
            }
        },
        dismissButton = {
            TextButton(onClick = safeDismiss, enabled = !inFlight) {
                Text(if (created != null) "Done" else "Cancel")
            }
        },
    )
}

/** Single-choice View/Edit (or Viewer/Editor) selector — one is always picked. */
@Composable
private fun PermissionSelector(
    selected: Permission,
    onSelect: (Permission) -> Unit,
    readLabel: String,
    writeLabel: String,
    modifier: Modifier = Modifier,
) {
    SingleChoiceSegmentedButtonRow(modifier = modifier) {
        SegmentedButton(
            selected = selected == Permission.READ,
            onClick = { onSelect(Permission.READ) },
            shape = SegmentedButtonDefaults.itemShape(index = 0, count = 2),
        ) { Text(readLabel) }
        SegmentedButton(
            selected = selected == Permission.WRITE,
            onClick = { onSelect(Permission.WRITE) },
            shape = SegmentedButtonDefaults.itemShape(index = 1, count = 2),
        ) { Text(writeLabel) }
    }
}

/** Human-readable date for the entered expiry, or null when blank/out of range. */
private fun resolvedExpiry(daysText: String): String? {
    val days = daysText.toIntOrNull()?.takeIf { it in 1..3650 } ?: return null
    val instant = Instant.now().plus(days.toLong(), ChronoUnit.DAYS)
    return SimpleDateFormat("MMM d, yyyy", Locale.getDefault()).format(Date(instant.toEpochMilli()))
}
