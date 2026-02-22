package handlers

import (
	"errors"
	"net/http"

	"github.com/Aditya-c-hu/Librarymanagement/internal/models"
	"github.com/Aditya-c-hu/Librarymanagement/internal/services"
)

type BookHandler struct {
	books *services.BookService
}

func NewBookHandler(books *services.BookService) *BookHandler {
	return &BookHandler{books: books}
}

func (h *BookHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req models.CreateBookRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Title == "" || req.Author == "" || req.ISBN == "" {
		writeError(w, http.StatusBadRequest, "title, author, and isbn are required")
		return
	}

	book, err := h.books.Create(req)
	if err != nil {
		if errors.Is(err, services.ErrDuplicateISBN) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, book)
}

func (h *BookHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := urlParamInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	book, err := h.books.GetByID(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "book not found")
		return
	}

	writeJSON(w, http.StatusOK, book)
}

func (h *BookHandler) List(w http.ResponseWriter, r *http.Request) {
	page := queryInt(r, "page", 1)
	perPage := queryInt(r, "per_page", 20)
	search := r.URL.Query().Get("search")

	result, err := h.books.List(page, perPage, search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *BookHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := urlParamInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	var req models.UpdateBookRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	book, err := h.books.Update(id, req)
	if err != nil {
		if errors.Is(err, services.ErrBookNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, book)
}

func (h *BookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := urlParamInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	if err := h.books.Delete(id); err != nil {
		if errors.Is(err, services.ErrBookNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, models.SuccessResponse{Message: "book deleted"})
}

func (h *BookHandler) AddCopies(w http.ResponseWriter, r *http.Request) {
	id, err := urlParamInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	var req struct {
		Count int `json:"count"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Count < 1 {
		writeError(w, http.StatusBadRequest, "count must be a positive integer")
		return
	}

	copies, err := h.books.AddCopies(id, req.Count)
	if err != nil {
		if errors.Is(err, services.ErrBookNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, copies)
}

func (h *BookHandler) GetCopies(w http.ResponseWriter, r *http.Request) {
	id, err := urlParamInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	copies, err := h.books.GetCopies(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, copies)
}

func (h *BookHandler) UpdateCopyStatus(w http.ResponseWriter, r *http.Request) {
	copyID, err := urlParamInt(r, "copyId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid copy id")
		return
	}

	var req struct {
		Status models.CopyStatus `json:"status"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.books.UpdateCopyStatus(copyID, req.Status); err != nil {
		if errors.Is(err, services.ErrCopyNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, models.SuccessResponse{Message: "copy status updated"})
}
