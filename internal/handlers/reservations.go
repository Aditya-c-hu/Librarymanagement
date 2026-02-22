package handlers

import (
	"errors"
	"net/http"

	"github.com/ravidesai/library-api/internal/middleware"
	"github.com/ravidesai/library-api/internal/models"
	"github.com/ravidesai/library-api/internal/services"
)

type ReservationHandler struct {
	reservations *services.ReservationService
}

func NewReservationHandler(reservations *services.ReservationService) *ReservationHandler {
	return &ReservationHandler{reservations: reservations}
}

func (h *ReservationHandler) Reserve(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req models.ReservationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.BookID == 0 {
		writeError(w, http.StatusBadRequest, "book_id is required")
		return
	}

	detail, err := h.reservations.Reserve(userID, req.BookID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrBookNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, services.ErrAlreadyReserved):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusCreated, detail)
}

func (h *ReservationHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	resID, err := urlParamInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid reservation id")
		return
	}

	userID := middleware.GetUserID(r.Context())
	isLib := middleware.IsLibrarian(r.Context())

	if err := h.reservations.Cancel(resID, userID, isLib); err != nil {
		switch {
		case errors.Is(err, services.ErrReservationNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, services.ErrCannotCancelFulfilled):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, models.SuccessResponse{Message: "reservation cancelled"})
}

func (h *ReservationHandler) GetMyReservations(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	list, err := h.reservations.ListByUser(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func (h *ReservationHandler) GetBookReservations(w http.ResponseWriter, r *http.Request) {
	bookID, err := urlParamInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid book id")
		return
	}

	list, err := h.reservations.ListByBook(bookID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func (h *ReservationHandler) GetAll(w http.ResponseWriter, r *http.Request) {
	list, err := h.reservations.ListAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, list)
}
