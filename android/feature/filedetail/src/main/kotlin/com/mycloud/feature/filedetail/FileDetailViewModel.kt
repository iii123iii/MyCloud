package com.mycloud.feature.filedetail

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.mycloud.core.common.result.NetworkResult
import com.mycloud.core.common.result.userMessageOrNull
import com.mycloud.core.common.util.FileKind
import com.mycloud.core.common.util.MimeType
import com.mycloud.core.model.Comment
import com.mycloud.core.model.FileNode
import com.mycloud.core.model.FileVersion
import com.mycloud.data.repository.CommentRepository
import com.mycloud.data.repository.FileRepository
import com.mycloud.data.repository.VersionRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.io.File
import javax.inject.Inject

data class FileDetailUiState(
    val comments: List<Comment> = emptyList(),
    val versions: List<FileVersion> = emptyList(),
    val newComment: String = "",
    val isLoading: Boolean = false,
    // True while a local copy is being downloaded for share/open.
    val isPreparingFile: Boolean = false,
    val errorMessage: String? = null,
)

/** Loading state of the PDF render source. */
sealed interface PdfState {
    data object Loading : PdfState
    data class Ready(val file: File) : PdfState
    data class Error(val message: String) : PdfState
}

@HiltViewModel
class FileDetailViewModel @Inject constructor(
    private val commentRepository: CommentRepository,
    private val versionRepository: VersionRepository,
    private val fileRepository: FileRepository,
) : ViewModel() {

    private val _state = MutableStateFlow(FileDetailUiState())
    val state: StateFlow<FileDetailUiState> = _state.asStateFlow()

    private val _pdf = MutableStateFlow<PdfState>(PdfState.Loading)
    val pdf: StateFlow<PdfState> = _pdf.asStateFlow()

    private var fileId: String? = null
    private var fileName: String? = null
    private var fileMime: String? = null
    private var isPdf: Boolean = false
    private var pdfJob: Job? = null
    private var prepareJob: Job? = null

    fun load(file: FileNode) {
        // Always (re)load on entry — LaunchedEffect(file.id) only fires this on a real
        // open, so re-opening a file shows fresh data and switching files never shows
        // the previous file's comments/versions/PDF.
        fileId = file.id
        fileName = file.name
        fileMime = file.mimeType
        isPdf = MimeType.kindOf(file.mimeType) == FileKind.PDF
        _pdf.value = PdfState.Loading
        _state.value = FileDetailUiState(isLoading = true)
        refresh()
        if (isPdf) loadPdf(file.id, file.name)
    }

    /** (Re)download the PDF to cache; exposes Ready/Error so the UI never spins forever. */
    fun loadPdf(id: String? = fileId, name: String? = fileName) {
        if (id == null || name == null) return
        pdfJob?.cancel()
        _pdf.value = PdfState.Loading
        pdfJob = viewModelScope.launch {
            val downloaded = try {
                fileRepository.downloadToCache(id, name)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                null
            }
            // Ignore a download that finished after the user already switched files.
            if (fileId != id) return@launch
            _pdf.value = if (downloaded != null) {
                PdfState.Ready(downloaded)
            } else {
                PdfState.Error("Couldn't download the document. Tap to retry.")
            }
        }
    }

    /**
     * Download a local copy of the current file to cache, then hand the [File] (and its mime,
     * defaulting to a generic binary type) to [onReady] so the caller can share/open it via the
     * existing FileProvider. Surfaces failures through [FileDetailUiState.errorMessage] and toggles
     * [FileDetailUiState.isPreparingFile] for the UI's progress/disabled state.
     */
    fun prepareLocalFile(onReady: (File, String) -> Unit) {
        val id = fileId ?: return
        val name = fileName ?: return
        if (_state.value.isPreparingFile) return
        prepareJob?.cancel()
        _state.update { it.copy(isPreparingFile = true) }
        prepareJob = viewModelScope.launch {
            val downloaded = try {
                fileRepository.downloadToCache(id, name)
            } catch (cancelled: CancellationException) {
                throw cancelled
            } catch (_: Exception) {
                null
            }
            // Ignore a download that finished after the user already switched files.
            if (fileId != id) return@launch
            _state.update { it.copy(isPreparingFile = false) }
            if (downloaded != null) {
                onReady(downloaded, fileMime ?: "application/octet-stream")
            } else {
                _state.update { it.copy(errorMessage = "Couldn't prepare the file. Please try again.") }
            }
        }
    }

    private fun refresh() {
        refreshComments()
        refreshVersions()
    }

    fun refreshComments() {
        val id = fileId ?: return
        _state.update { it.copy(isLoading = true) }
        viewModelScope.launch {
            when (val result = commentRepository.list(id)) {
                is NetworkResult.Success ->
                    _state.update { it.copy(comments = result.data, isLoading = false, errorMessage = null) }
                else ->
                    _state.update { it.copy(isLoading = false, errorMessage = result.userMessageOrNull()) }
            }
        }
    }

    fun refreshVersions() {
        val id = fileId ?: return
        viewModelScope.launch {
            when (val result = versionRepository.list(id)) {
                is NetworkResult.Success ->
                    _state.update { it.copy(versions = result.data) }
                else ->
                    _state.update { it.copy(errorMessage = result.userMessageOrNull()) }
            }
        }
    }

    fun setNewComment(value: String) = _state.update { it.copy(newComment = value) }

    fun clearError() = _state.update { it.copy(errorMessage = null) }

    fun addComment() {
        val id = fileId ?: return
        val text = _state.value.newComment.trim()
        if (text.isEmpty()) return
        viewModelScope.launch {
            when (val result = commentRepository.add(id, text)) {
                is NetworkResult.Success -> {
                    // Only clear the draft once the server accepted it.
                    _state.update { it.copy(newComment = "") }
                    refreshComments()
                }
                else -> _state.update { it.copy(errorMessage = result.userMessageOrNull()) }
            }
        }
    }

    fun editComment(commentId: String, content: String) {
        val text = content.trim()
        if (text.isEmpty()) return
        viewModelScope.launch {
            when (val result = commentRepository.edit(commentId, text)) {
                is NetworkResult.Success -> refreshComments()
                else -> _state.update { it.copy(errorMessage = result.userMessageOrNull()) }
            }
        }
    }

    fun deleteComment(commentId: String) {
        viewModelScope.launch {
            when (val result = commentRepository.delete(commentId)) {
                is NetworkResult.Success -> refreshComments()
                else -> _state.update { it.copy(errorMessage = result.userMessageOrNull()) }
            }
        }
    }

    fun restoreVersion(versionNumber: Int) {
        val id = fileId ?: return
        viewModelScope.launch {
            when (val result = versionRepository.restore(id, versionNumber)) {
                is NetworkResult.Success -> refreshVersions()
                else -> _state.update { it.copy(errorMessage = result.userMessageOrNull()) }
            }
        }
    }

    fun previewUrl(id: String): String = fileRepository.previewUrl(id)
}
