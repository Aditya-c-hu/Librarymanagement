package handlers

import (
	"errors"
	"net/http"

	"github.com/ravidesai/library-api/internal/middleware"
	"github.com/ravidesai/library-api/internal/models"
	"github.com/ravidesai/library-api/internal/services"
)

type CheckoutHandler struct {
	checkouts *services.CheckoutService
}

func NewCheckoutHandler(checkouts *services.CheckoutService) *CheckoutHandler {
	return &CheckoutHandler{checkouts: checkouts}
}

func (h *CheckoutHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req models.CheckoutRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.BookID == 0 {
		writeError(w, http.StatusBadRequest, "book_id is required")
		return
	}

	detail, err := h.checkouts.CheckoutBook(userID, req.BookID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrBookNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, services.ErrAlreadyCheckedOut):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, services.ErrNoCopiesAvailable):
			writeError(w, http.StatusConflict, "no copies available — consider placing a reservation")
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusCreated, detail)
}

func (h *CheckoutHandler) Return(w http.ResponseWriter, r *http.Request) {
	checkoutID, err := urlParamInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid checkout id")
		return
	}

	userID := middleware.GetUserID(r.Context())
	isLib := middleware.IsLibrarian(r.Context())

	resp, err := h.checkouts.ReturnBook(checkoutID, userID, isLib)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrCheckoutNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, services.ErrAlreadyReturned):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *CheckoutHandler) Renew(w http.ResponseWriter, r *http.Request) {
	checkoutID, err := urlParamInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid checkout id")
		return
	}

	userID := middleware.GetUserID(r.Context())

	detail, err := h.checkouts.RenewCheckout(checkoutID, userID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrCheckoutNotFound):
			writeError(w, http.StatusNotFound, err.Error())
		case errors.Is(err, services.ErrAlreadyReturned):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, services.ErrMaxRenewalsReached):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, services.ErrCannotRenewOverdue):
			writeError(w, http.StatusConflict, err.Error())
		default:
			writeError(w, http.StatusConflict, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

func (h *CheckoutHandler) GetMyCheckouts(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	activeOnly := r.URL.Query().Get("active") == "true"

	list, err := h.checkouts.ListByUser(userID, activeOnly)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func (h *CheckoutHandler) GetAllCheckouts(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("active") == "true"

	list, err := h.checkouts.ListAll(activeOnly)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func (h *CheckoutHandler) GetOverdue(w http.ResponseWriter, r *http.Request) {
	list, err := h.checkouts.ListOverdue()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func (h *CheckoutHandler) PayFine(w http.ResponseWriter, r *http.Request) {
	checkoutID, err := urlParamInt(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid checkout id")
		return
	}

	if err := h.checkouts.PayFine(checkoutID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, models.SuccessResponse{Message: "fine paid"})
}
