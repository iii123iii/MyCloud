package com.mycloud.feature.sharing

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.mycloud.core.model.Permission

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
    val created = state.createdLink

    // Clear any state left over from a previously-shared item (the VM is reused).
    LaunchedEffect(fileId, folderId) { viewModel.reset() }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(if (created == null) "Share “$title”" else "Link ready") },
        text = {
            if (created != null) {
                Text(created, style = MaterialTheme.typography.bodyMedium)
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
                        modifier = Modifier.fillMaxWidth(),
                    )
                    Row(
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                    ) {
                        FilterChip(
                            selected = state.grantPermission == Permission.READ,
                            onClick = { viewModel.setGrantPermission(Permission.READ) },
                            label = { Text("Viewer") },
                        )
                        FilterChip(
                            selected = state.grantPermission == Permission.WRITE,
                            onClick = { viewModel.setGrantPermission(Permission.WRITE) },
                            label = { Text("Editor") },
                        )
                        TextButton(
                            onClick = { viewModel.addPerson(fileId, folderId) },
                            enabled = state.grantee.isNotBlank() && !state.isGranting,
                            modifier = Modifier.weight(1f),
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
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        FilterChip(
                            selected = state.permission == Permission.READ,
                            onClick = { viewModel.setPermission(Permission.READ) },
                            label = { Text("View") },
                        )
                        FilterChip(
                            selected = state.permission == Permission.WRITE,
                            onClick = { viewModel.setPermission(Permission.WRITE) },
                            label = { Text("Edit") },
                        )
                    }
                    OutlinedTextField(
                        value = state.password,
                        onValueChange = viewModel::setPassword,
                        label = { Text("Password (optional)") },
                        singleLine = true,
                        visualTransformation = PasswordVisualTransformation(),
                        modifier = Modifier.fillMaxWidth(),
                    )
                    OutlinedTextField(
                        value = state.expiresInDays,
                        onValueChange = viewModel::setExpiresInDays,
                        label = { Text("Expires in days (optional)") },
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
                TextButton(onClick = { clipboard.setText(AnnotatedString(created)) }) { Text("Copy link") }
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
            TextButton(onClick = onDismiss) { Text(if (created != null) "Done" else "Cancel") }
        },
    )
}
