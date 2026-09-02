package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"homeflow/backend/internal/auth"
	"homeflow/backend/internal/calendar"
	"homeflow/backend/internal/documents"
	"homeflow/backend/internal/expenses"
	"homeflow/backend/internal/links"
	"homeflow/backend/internal/observability"
	"homeflow/backend/internal/pets"
	"homeflow/backend/internal/receipts"
	"homeflow/backend/internal/release"
	"homeflow/backend/internal/reminders"
	"homeflow/backend/internal/shopping"
	"homeflow/backend/internal/vets"
)

// Version ist die Release, mit der dieses Binary gebaut wurde. cmd/homeflow
// setzt sie beim Start aus dem über -ldflags eingespeisten Wert; ohne das steht
// hier "unbekannt", und genau das ist die ehrliche Antwort.
var Version = "unbekannt"

type Server struct {
	logger    *slog.Logger
	shopping  *shopping.Service
	expenses  *expenses.Service
	receipts  *receipts.Service
	calendar  *calendar.Service
	pets      *pets.Service
	documents *documents.Service
	reminders *reminders.Service
	links     *links.Service
	vets      *vets.Service
	sessions  SessionService
	limiter   *loginRateLimiter
	handler   http.Handler
	cookies   BrowserCookieConfig
}

const (
	accessCookie  = "homeflow_access"
	refreshCookie = "homeflow_refresh"
	csrfCookie    = "homeflow_csrf"
)

type BrowserCookieConfig struct {
	Secure  bool
	WebRoot string
	Metrics *observability.Metrics
	// Calendar is optional: without it the calendar routes are not registered
	// at all, in the same way the expense and receipt routes are conditional.
	Calendar  *calendar.Service
	Pets      *pets.Service
	Documents *documents.Service
	Reminders *reminders.Service
	Links     *links.Service
	Vets      *vets.Service
}

type SessionService interface {
	Login(context.Context, auth.Credentials, string) (auth.TokenPair, error)
	Refresh(context.Context, string, string) (auth.TokenPair, error)
	Logout(context.Context, string, string) error
}

func New(logger *slog.Logger, authenticator auth.Authenticator, shoppingService *shopping.Service) *Server {
	return NewWithSessions(logger, authenticator, nil, shoppingService)
}

func NewWithSessions(logger *slog.Logger, authenticator auth.Authenticator, sessions SessionService, shoppingService *shopping.Service) *Server {
	return NewFull(logger, authenticator, sessions, shoppingService, nil, nil)
}

func NewFull(logger *slog.Logger, authenticator auth.Authenticator, sessions SessionService, shoppingService *shopping.Service, expenseService *expenses.Service, receiptService *receipts.Service) *Server {
	return NewFullWithOptions(logger, authenticator, sessions, shoppingService, expenseService, receiptService, BrowserCookieConfig{})
}

func NewFullWithOptions(logger *slog.Logger, authenticator auth.Authenticator, sessions SessionService, shoppingService *shopping.Service, expenseService *expenses.Service, receiptService *receipts.Service, options BrowserCookieConfig) *Server {
	s := &Server{logger: logger, shopping: shoppingService, expenses: expenseService, receipts: receiptService,
		calendar: options.Calendar, pets: options.Pets, documents: options.Documents, reminders: options.Reminders, links: options.Links, vets: options.Vets, sessions: sessions, limiter: newLoginRateLimiter(5, 15*time.Minute), cookies: options}

	public := http.NewServeMux()
	public.HandleFunc("GET /health", s.health)
	if options.Metrics != nil {
		public.Handle("GET /metrics", options.Metrics.Handler())
	}
	if sessions != nil {
		public.HandleFunc("POST /api/v1/auth/login", s.login)
		public.HandleFunc("POST /api/v1/auth/refresh", s.refresh)
		public.HandleFunc("POST /api/v1/auth/logout", s.logout)
		public.HandleFunc("POST /api/v1/auth/browser/login", s.browserLogin)
		public.HandleFunc("POST /api/v1/auth/browser/refresh", s.browserRefresh)
		public.Handle("POST /api/v1/auth/browser/logout", s.csrf(http.HandlerFunc(s.browserLogout)))
	}

	private := http.NewServeMux()
	// Hinter der Anmeldung, nicht davor: die Notizen tragen Ticketnamen, und die
	// beschreiben ein Haushaltssystem. Sie werden nach dem Anmelden angezeigt,
	// also reicht das (HF-0105).
	private.HandleFunc("GET /api/v1/version", s.versionInfo)
	private.HandleFunc("GET /api/v1/shopping/items", s.listItems)
	private.HandleFunc("GET /api/v1/shopping/lists", s.listShoppingLists)
	private.HandleFunc("POST /api/v1/shopping/items", s.createItem)
	private.HandleFunc("POST /api/v1/shopping/items/{itemID}/complete", s.completeItem)
	if expenseService != nil {
		private.HandleFunc("GET /api/v1/expenses", s.listExpenses)
		private.HandleFunc("POST /api/v1/expenses", s.createExpense)
		private.HandleFunc("POST /api/v1/expenses/{expenseID}/paid", s.markExpensePaid)
		private.HandleFunc("GET /api/v1/expense-summaries/{month}", s.expenseSummary)
		private.HandleFunc("GET /api/v1/pets/{petID}/budgets", s.listPetBudgets)
		private.HandleFunc("PUT /api/v1/pets/{petID}/budgets", s.upsertPetBudget)
		private.HandleFunc("GET /api/v1/pets/{petID}/expense-summary", s.petExpenseSummary)
	}
	if receiptService != nil {
		private.HandleFunc("GET /api/v1/expenses/{expenseID}/receipts", s.listReceipts)
		private.HandleFunc("POST /api/v1/expenses/{expenseID}/receipts", s.uploadReceipt)
		private.HandleFunc("GET /api/v1/expenses/{expenseID}/receipts/{receiptID}/content", s.downloadReceipt)
	}
	if options.Calendar != nil {
		private.HandleFunc("GET /api/v1/calendar/events", s.listCalendarEvents)
		private.HandleFunc("POST /api/v1/calendar/events", s.createCalendarEvent)
		private.HandleFunc("GET /api/v1/calendar/events/{eventID}", s.getCalendarEvent)
		private.HandleFunc("PUT /api/v1/calendar/events/{eventID}", s.updateCalendarEvent)
		private.HandleFunc("DELETE /api/v1/calendar/events/{eventID}", s.deleteCalendarEvent)
	}
	if options.Pets != nil {
		private.HandleFunc("GET /api/v1/pets", s.listPets)
		private.HandleFunc("POST /api/v1/pets", s.createPet)
		private.HandleFunc("GET /api/v1/pets/{petID}", s.getPet)
		private.HandleFunc("PATCH /api/v1/pets/{petID}", s.updatePet)
		private.HandleFunc("GET /api/v1/pets/{petID}/medical-records", s.listMedicalRecords)
		private.HandleFunc("POST /api/v1/pets/{petID}/medical-records", s.createMedicalRecord)
		private.HandleFunc("GET /api/v1/pets/{petID}/vaccinations", s.listVaccinations)
		private.HandleFunc("POST /api/v1/pets/{petID}/vaccinations", s.createVaccination)
		private.HandleFunc("GET /api/v1/pets/{petID}/medications", s.listPetMedications)
		private.HandleFunc("POST /api/v1/pets/{petID}/medications", s.createPetMedication)
		private.HandleFunc("GET /api/v1/pets/{petID}/medications/{medicationID}/restock-preview", s.medicationRestockPreview)
		private.HandleFunc("POST /api/v1/pets/{petID}/medications/{medicationID}/shopping-item", s.createMedicationShoppingItem)
		private.HandleFunc("PUT /api/v1/pets/{petID}/medications/{medicationID}/stock", s.updateMedicationStock)
		private.HandleFunc("GET /api/v1/pets/{petID}/weights", s.listWeights)
		private.HandleFunc("POST /api/v1/pets/{petID}/weights", s.createWeight)
		private.HandleFunc("GET /api/v1/pets/{petID}/lab-results", s.listLabResults)
		private.HandleFunc("POST /api/v1/pets/{petID}/lab-results", s.createLabResult)
		private.HandleFunc("GET /api/v1/pets/{petID}/dashboard", s.petDashboard)
		private.HandleFunc("GET /api/v1/pets/{petID}/timeline", s.petTimeline)
	}
	if options.Vets != nil {
		private.HandleFunc("GET /api/v1/veterinarians", s.listVeterinarians)
		private.HandleFunc("POST /api/v1/veterinarians", s.createVeterinarian)
		private.HandleFunc("GET /api/v1/veterinarians/{vetID}", s.getVeterinarian)
		private.HandleFunc("PATCH /api/v1/veterinarians/{vetID}", s.updateVeterinarian)
		private.HandleFunc("DELETE /api/v1/veterinarians/{vetID}", s.deleteVeterinarian)
	}
	if options.Reminders != nil {
		private.HandleFunc("GET /api/v1/reminders", s.listReminders)
		private.HandleFunc("GET /api/v1/reminder-rules", s.getReminderRules)
		private.HandleFunc("PUT /api/v1/reminder-rules", s.replaceReminderRules)
		private.HandleFunc("POST /api/v1/reminders/vaccinations/{sourceID}/appointment", s.createReminderAppointment)
	}
	if options.Documents != nil {
		private.HandleFunc("GET /api/v1/pets/{petID}/documents", s.listDocuments)
		private.HandleFunc("POST /api/v1/pets/{petID}/documents", s.uploadDocument)
		private.HandleFunc("GET /api/v1/pets/{petID}/documents/{documentID}/content", s.downloadDocument)
	}
	if options.Links != nil {
		private.HandleFunc("GET /api/v1/pets/{petID}/documents/{documentID}/links", s.getDocumentLinks)
		private.HandleFunc("PUT /api/v1/pets/{petID}/documents/{documentID}/links", s.replaceDocumentLinks)
		private.HandleFunc("GET /api/v1/expenses/{expenseID}/pet-links", s.getExpenseLinks)
		private.HandleFunc("PUT /api/v1/expenses/{expenseID}/pet-links", s.replaceExpenseLinks)
	}
	public.Handle("/api/", s.csrf(auth.BrowserMiddleware(authenticator, accessCookie, private)))
	if options.WebRoot != "" {
		public.Handle("/", spaHandler(options.WebRoot))
	}

	s.handler = securityHeaders(requestLogger(logger, options.Metrics, recoverer(logger, public)))
	return s
}

func (s *Server) medicationRestockPreview(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	preview, err := s.shopping.PreviewMedication(r.Context(), principal.UserID, r.PathValue("petID"), r.PathValue("medicationID"))
	switch {
	case errors.Is(err, shopping.ErrInvalid):
		writeProblem(w, http.StatusBadRequest, "Invalid medication id")
	case errors.Is(err, shopping.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Medication not found")
	case errors.Is(err, shopping.ErrNotLow):
		writeProblem(w, http.StatusConflict, "Stock is not below minimum")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, preview)
	}
}
func (s *Server) createMedicationShoppingItem(w http.ResponseWriter, r *http.Request) {
	var input shopping.MedicationRestockInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	item, err := s.shopping.CreateMedicationItem(r.Context(), principal.UserID, r.PathValue("petID"), r.PathValue("medicationID"), input)
	switch {
	case errors.Is(err, shopping.ErrInvalid):
		writeProblem(w, http.StatusBadRequest, "Invalid medication id")
	case errors.Is(err, shopping.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Medication or list not found")
	case errors.Is(err, shopping.ErrDuplicate):
		writeProblem(w, http.StatusConflict, "Open shopping item already exists")
	case err != nil:
		s.internalError(w, err)
	default:
		w.Header().Set("Location", "/api/v1/shopping/items/"+item.ID)
		writeJSON(w, http.StatusCreated, item)
	}
}
func (s *Server) updateMedicationStock(w http.ResponseWriter, r *http.Request) {
	var input pets.MedicationStockInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	item, err := s.pets.UpdateMedicationStock(r.Context(), principal.UserID, r.PathValue("petID"), r.PathValue("medicationID"), input)
	switch {
	case errors.Is(err, pets.ErrInvalidMedication):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid stock quantity")
	case errors.Is(err, pets.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Medication not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, item)
	}
}

func (s *Server) createReminderAppointment(w http.ResponseWriter, r *http.Request) {
	var input reminders.AppointmentInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	event, err := s.reminders.CreateAppointment(r.Context(), principal.UserID, r.PathValue("sourceID"), input)
	switch {
	case errors.Is(err, reminders.ErrInvalid):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid appointment")
	case errors.Is(err, reminders.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Reminder source not found")
	case errors.Is(err, reminders.ErrDuplicate):
		writeProblem(w, http.StatusConflict, "Appointment already exists")
	case err != nil:
		s.internalError(w, err)
	default:
		w.Header().Set("Location", "/api/v1/calendar/events/"+event.ID)
		writeJSON(w, http.StatusCreated, event)
	}
}

func (s *Server) getReminderRules(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	rules, err := s.reminders.GetRules(r.Context(), principal.UserID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (s *Server) replaceReminderRules(w http.ResponseWriter, r *http.Request) {
	var input reminders.Rules
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	rules, err := s.reminders.ReplaceRules(r.Context(), principal.UserID, input)
	if errors.Is(err, reminders.ErrInvalid) {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid reminder rules")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

func (s *Server) petTimeline(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), strconv.Itoa(pets.TimelineDefaultLimit)))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid timeline query")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	page, err := s.pets.ListTimeline(r.Context(), principal.UserID, r.PathValue("petID"), limit, r.URL.Query().Get("cursor"))
	switch {
	case errors.Is(err, pets.ErrInvalidID), errors.Is(err, pets.ErrInvalidTimeline):
		writeProblem(w, http.StatusBadRequest, "Invalid timeline query")
	case errors.Is(err, pets.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, page)
	}
}

func (s *Server) listPetMedications(w http.ResponseWriter, r *http.Request) {
	includeInactive, err := strconv.ParseBool(defaultString(r.URL.Query().Get("includeInactive"), "false"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid query parameter")
		return
	}
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), strconv.Itoa(pets.PetMedicationMaxList)))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid query parameter")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	page, err := s.pets.ListPetMedications(r.Context(), principal.UserID, r.PathValue("petID"), includeInactive, limit)
	switch {
	case errors.Is(err, pets.ErrInvalidID), errors.Is(err, pets.ErrInvalidMedication):
		writeProblem(w, http.StatusBadRequest, "Invalid medication query")
	case errors.Is(err, pets.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, page)
	}
}

func (s *Server) petDashboard(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	dashboard, err := s.pets.Dashboard(r.Context(), principal.UserID, r.PathValue("petID"), r.URL.Query().Get("month"))
	switch {
	case errors.Is(err, pets.ErrInvalidID), errors.Is(err, pets.ErrInvalid):
		writeProblem(w, http.StatusBadRequest, "Invalid pet or month")
	case errors.Is(err, pets.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, dashboard)
	}
}

func (s *Server) listWeights(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), "100"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid limit")
		return
	}
	items, err := s.pets.ListWeights(r.Context(), principal.UserID, r.PathValue("petID"), limit)
	s.writeMeasurementList(w, map[string]any{"weights": items}, err)
}
func (s *Server) createWeight(w http.ResponseWriter, r *http.Request) {
	var input pets.WeightInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	item, err := s.pets.CreateWeight(r.Context(), principal.UserID, r.PathValue("petID"), input)
	s.writeMeasurement(w, item, err)
}
func (s *Server) listLabResults(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), "100"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid limit")
		return
	}
	items, err := s.pets.ListLabResults(r.Context(), principal.UserID, r.PathValue("petID"), limit)
	s.writeMeasurementList(w, map[string]any{"labResults": items}, err)
}
func (s *Server) createLabResult(w http.ResponseWriter, r *http.Request) {
	var input pets.LabResultInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	item, err := s.pets.CreateLabResult(r.Context(), principal.UserID, r.PathValue("petID"), input)
	s.writeMeasurement(w, item, err)
}
func (s *Server) writeMeasurementList(w http.ResponseWriter, value any, err error) {
	switch {
	case errors.Is(err, pets.ErrInvalidID), errors.Is(err, pets.ErrInvalidMeasurement):
		writeProblem(w, http.StatusBadRequest, "Invalid measurement query")
	case errors.Is(err, pets.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, value)
	}
}
func (s *Server) writeMeasurement(w http.ResponseWriter, value any, err error) {
	switch {
	case errors.Is(err, pets.ErrInvalidID):
		writeProblem(w, http.StatusBadRequest, "Invalid pet id")
	case errors.Is(err, pets.ErrInvalidMeasurement):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid measurement")
	case errors.Is(err, pets.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusCreated, value)
	}
}

func (s *Server) listReminders(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), "100"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid limit")
		return
	}
	items, err := s.reminders.List(r.Context(), principal.UserID, r.URL.Query().Get("until"), limit)
	switch {
	case errors.Is(err, reminders.ErrInvalid):
		writeProblem(w, http.StatusBadRequest, "Invalid reminder range")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"reminders": items})
	}
}
func (s *Server) createPetMedication(w http.ResponseWriter, r *http.Request) {
	var input pets.PetMedicationInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	medication, err := s.pets.CreatePetMedication(r.Context(), principal.UserID, r.PathValue("petID"), input)
	switch {
	case errors.Is(err, pets.ErrInvalidID):
		writeProblem(w, http.StatusBadRequest, "Invalid pet id")
	case errors.Is(err, pets.ErrInvalidMedication):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid medication")
	case errors.Is(err, pets.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		w.Header().Set("Location", "/api/v1/pets/"+medication.PetID+"/medications/"+medication.ID)
		writeJSON(w, http.StatusCreated, medication)
	}
}

func (s *Server) listVaccinations(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), strconv.Itoa(pets.VaccinationDefaultLimit)))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid query parameter")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	page, err := s.pets.ListVaccinations(r.Context(), principal.UserID, r.PathValue("petID"), limit, r.URL.Query().Get("cursor"))
	switch {
	case errors.Is(err, pets.ErrInvalidID), errors.Is(err, pets.ErrInvalidVaccination):
		writeProblem(w, http.StatusBadRequest, "Invalid vaccination query")
	case errors.Is(err, pets.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, page)
	}
}

func (s *Server) createVaccination(w http.ResponseWriter, r *http.Request) {
	var input pets.VaccinationInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	vaccination, err := s.pets.CreateVaccination(r.Context(), principal.UserID, r.PathValue("petID"), input)
	switch {
	case errors.Is(err, pets.ErrInvalidID):
		writeProblem(w, http.StatusBadRequest, "Invalid pet id")
	case errors.Is(err, pets.ErrInvalidVaccination):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid vaccination")
	case errors.Is(err, pets.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		w.Header().Set("Location", "/api/v1/pets/"+vaccination.PetID+"/vaccinations/"+vaccination.ID)
		writeJSON(w, http.StatusCreated, vaccination)
	}
}

func (s *Server) listMedicalRecords(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), strconv.Itoa(pets.MedicalRecordDefaultLimit)))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid query parameter")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	page, err := s.pets.ListMedicalRecords(r.Context(), principal.UserID, r.PathValue("petID"), limit, r.URL.Query().Get("cursor"))
	switch {
	case errors.Is(err, pets.ErrInvalidID), errors.Is(err, pets.ErrInvalidRecord):
		writeProblem(w, http.StatusBadRequest, "Invalid medical record query")
	case errors.Is(err, pets.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, page)
	}
}

func (s *Server) createMedicalRecord(w http.ResponseWriter, r *http.Request) {
	var input pets.MedicalRecordInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	record, err := s.pets.CreateMedicalRecord(r.Context(), principal.UserID, r.PathValue("petID"), input)
	switch {
	case errors.Is(err, pets.ErrInvalidID):
		writeProblem(w, http.StatusBadRequest, "Invalid pet id")
	case errors.Is(err, pets.ErrInvalidRecord):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid medical record")
	case errors.Is(err, pets.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		w.Header().Set("Location", "/api/v1/pets/"+record.PetID+"/medical-records/"+record.ID)
		writeJSON(w, http.StatusCreated, record)
	}
}

func (s *Server) listPets(w http.ResponseWriter, r *http.Request) {
	includeInactive, err := strconv.ParseBool(defaultString(r.URL.Query().Get("includeInactive"), "false"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid query parameter")
		return
	}
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), strconv.Itoa(pets.DefaultLimit)))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid query parameter")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	page, err := s.pets.List(r.Context(), principal.UserID, includeInactive, limit, r.URL.Query().Get("after"))
	if errors.Is(err, pets.ErrInvalid) {
		writeProblem(w, http.StatusBadRequest, "Invalid query parameter")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) createPet(w http.ResponseWriter, r *http.Request) {
	var input pets.Input
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	pet, err := s.pets.Create(r.Context(), principal.UserID, input)
	if errors.Is(err, pets.ErrInvalid) {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid pet")
		return
	}
	if errors.Is(err, pets.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "Household not found")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/pets/"+pet.ID)
	writeJSON(w, http.StatusCreated, pet)
}

func (s *Server) getPet(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	pet, err := s.pets.Get(r.Context(), principal.UserID, r.PathValue("petID"))
	s.writePet(w, pet, err)
}

func (s *Server) updatePet(w http.ResponseWriter, r *http.Request) {
	var input pets.Input
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	pet, err := s.pets.Update(r.Context(), principal.UserID, r.PathValue("petID"), input)
	s.writePet(w, pet, err)
}

func (s *Server) writePet(w http.ResponseWriter, pet pets.Pet, err error) {
	switch {
	case errors.Is(err, pets.ErrInvalidID):
		writeProblem(w, http.StatusBadRequest, "Invalid pet id")
	case errors.Is(err, pets.ErrInvalid):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid pet")
	case errors.Is(err, pets.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, pet)
	}
}

func (s *Server) listVeterinarians(w http.ResponseWriter, r *http.Request) {
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), "50"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid query parameter")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	var page vets.Page
	if practice := r.URL.Query().Get("practice"); practice != "" {
		matches, matchErr := s.vets.Match(r.Context(), principal.UserID, practice, r.URL.Query().Get("postalCode"))
		if matchErr != nil {
			s.writeVetError(w, matchErr)
			return
		}
		page.Veterinarians = matches
	} else {
		page, err = s.vets.List(r.Context(), principal.UserID, r.URL.Query().Get("suche"), limit, r.URL.Query().Get("after"))
		if err != nil {
			s.writeVetError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, page)
}
func (s *Server) createVeterinarian(w http.ResponseWriter, r *http.Request) {
	var input vets.Input
	if decodeJSON(w, r, &input) != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	v, err := s.vets.Create(r.Context(), principal.UserID, input)
	if err != nil {
		s.writeVetError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/veterinarians/"+v.ID)
	writeJSON(w, http.StatusCreated, v)
}
func (s *Server) getVeterinarian(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	v, err := s.vets.Get(r.Context(), principal.UserID, r.PathValue("vetID"))
	if err != nil {
		s.writeVetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
func (s *Server) updateVeterinarian(w http.ResponseWriter, r *http.Request) {
	var input vets.Input
	if decodeJSON(w, r, &input) != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	v, err := s.vets.Update(r.Context(), principal.UserID, r.PathValue("vetID"), input)
	if err != nil {
		s.writeVetError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}
func (s *Server) deleteVeterinarian(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	if err := s.vets.Delete(r.Context(), principal.UserID, r.PathValue("vetID")); err != nil {
		s.writeVetError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) writeVetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, vets.ErrInvalid):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid veterinarian")
	case errors.Is(err, vets.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Veterinarian not found")
	case errors.Is(err, vets.ErrReferenced):
		writeProblem(w, http.StatusConflict, "Veterinarian is referenced")
	default:
		s.internalError(w, err)
	}
}

func (s *Server) browserLogin(w http.ResponseWriter, r *http.Request) {
	var credentials auth.Credentials
	if err := decodeJSON(w, r, &credentials); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	sourceIP := remoteIP(r)
	key := sourceIP + "|" + strings.ToLower(credentials.Username)
	if !s.limiter.Allow(key) {
		if s.cookies.Metrics != nil {
			s.cookies.Metrics.LoginFailed()
		}
		w.Header().Set("Retry-After", "900")
		writeProblem(w, http.StatusTooManyRequests, "Too many login attempts")
		return
	}
	pair, err := s.sessions.Login(r.Context(), credentials, sourceIP)
	if errors.Is(err, auth.ErrUnauthenticated) {
		if s.cookies.Metrics != nil {
			s.cookies.Metrics.LoginFailed()
		}
		writeProblem(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.limiter.Reset(key)
	csrf, err := randomBrowserToken()
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.setBrowserCookies(w, pair, csrf)
	writeJSON(w, http.StatusOK, map[string]string{"username": credentials.Username, "csrfToken": csrf})
}

func (s *Server) browserRefresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookie)
	if err != nil || cookie.Value == "" {
		writeProblem(w, http.StatusUnauthorized, "Invalid refresh token")
		return
	}
	pair, err := s.sessions.Refresh(r.Context(), cookie.Value, remoteIP(r))
	if errors.Is(err, auth.ErrUnauthenticated) {
		s.clearBrowserCookies(w)
		writeProblem(w, http.StatusUnauthorized, "Invalid refresh token")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	csrf, err := randomBrowserToken()
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.setBrowserCookies(w, pair, csrf)
	writeJSON(w, http.StatusOK, map[string]string{"csrfToken": csrf})
}

func (s *Server) browserLogout(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(refreshCookie)
	if cookie != nil && cookie.Value != "" {
		if err := s.sessions.Logout(r.Context(), cookie.Value, remoteIP(r)); err != nil {
			s.internalError(w, err)
			return
		}
	}
	s.clearBrowserCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setBrowserCookies(w http.ResponseWriter, pair auth.TokenPair, csrf string) {
	http.SetCookie(w, &http.Cookie{Name: accessCookie, Value: pair.AccessToken, Path: "/", HttpOnly: true, Secure: s.cookies.Secure, SameSite: http.SameSiteStrictMode, Expires: pair.AccessExpiresAt})
	http.SetCookie(w, &http.Cookie{Name: refreshCookie, Value: pair.RefreshToken, Path: "/api/v1/auth/browser", HttpOnly: true, Secure: s.cookies.Secure, SameSite: http.SameSiteStrictMode, Expires: pair.RefreshExpiresAt})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: csrf, Path: "/", HttpOnly: false, Secure: s.cookies.Secure, SameSite: http.SameSiteStrictMode, Expires: pair.RefreshExpiresAt})
}

func (s *Server) clearBrowserCookies(w http.ResponseWriter) {
	for _, cookie := range []*http.Cookie{
		{Name: accessCookie, Path: "/", HttpOnly: true},
		{Name: refreshCookie, Path: "/api/v1/auth/browser", HttpOnly: true},
		{Name: csrfCookie, Path: "/"},
	} {
		cookie.Secure = s.cookies.Secure
		cookie.SameSite = http.SameSiteStrictMode
		cookie.MaxAge = -1
		cookie.Expires = time.Unix(1, 0)
		http.SetCookie(w, cookie)
	}
}

func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions || strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			next.ServeHTTP(w, r)
			return
		}
		_, hasAccessCookie := r.Cookie(accessCookie)
		_, hasRefreshCookie := r.Cookie(refreshCookie)
		if hasAccessCookie != nil && hasRefreshCookie != nil {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(csrfCookie)
		header := r.Header.Get("X-CSRF-Token")
		if err != nil || cookie.Value == "" || header == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
			writeProblem(w, http.StatusForbidden, "Invalid CSRF token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func randomBrowserToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func spaHandler(root string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		path := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			http.NotFound(w, r)
			return
		}
		if path == "." {
			path = "index.html"
		}
		candidate := filepath.Join(root, path)
		if info, err := os.Stat(candidate); err != nil || info.IsDir() {
			candidate = filepath.Join(root, "index.html")
		}
		http.ServeFile(w, r, candidate)
	})
}

func (s *Server) listExpenses(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), "100"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid limit")
		return
	}
	items, err := s.expenses.List(r.Context(), principal.UserID, r.URL.Query().Get("month"), r.URL.Query().Get("petId"), limit)
	if errors.Is(err, expenses.ErrInvalid) {
		writeProblem(w, http.StatusBadRequest, "Invalid month")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"expenses": items})
}

func (s *Server) createExpense(w http.ResponseWriter, r *http.Request) {
	var input expenses.CreateExpense
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	expense, err := s.expenses.Create(r.Context(), input, principal.UserID)
	if errors.Is(err, expenses.ErrInvalid) {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid expense")
		return
	}
	if errors.Is(err, expenses.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "Pet not found")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/expenses/"+expense.ID)
	if s.cookies.Metrics != nil {
		s.cookies.Metrics.ExpenseCreated()
	}
	writeJSON(w, http.StatusCreated, expense)
}

func (s *Server) markExpensePaid(w http.ResponseWriter, r *http.Request) {
	var input expenses.MarkPaid
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	expense, err := s.expenses.MarkPaid(r.Context(), r.PathValue("expenseID"), principal.UserID, input)
	switch {
	case errors.Is(err, expenses.ErrInvalid):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid payment")
	case errors.Is(err, expenses.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Expense not found")
	case errors.Is(err, expenses.ErrConflict):
		writeProblem(w, http.StatusConflict, "Expense cannot be paid in its current state")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, expense)
	}
}

func (s *Server) expenseSummary(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	summary, err := s.expenses.MonthlySummary(r.Context(), principal.UserID, r.PathValue("month"))
	if errors.Is(err, expenses.ErrInvalid) {
		writeProblem(w, http.StatusBadRequest, "Invalid month")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) listPetBudgets(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	items, err := s.expenses.ListBudgets(r.Context(), principal.UserID, r.PathValue("petID"), r.URL.Query().Get("month"))
	switch {
	case errors.Is(err, expenses.ErrInvalid):
		writeProblem(w, http.StatusBadRequest, "Invalid pet or month")
	case errors.Is(err, expenses.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"budgets": items})
	}
}

func (s *Server) upsertPetBudget(w http.ResponseWriter, r *http.Request) {
	var input expenses.BudgetInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	budget, err := s.expenses.UpsertBudget(r.Context(), principal.UserID, r.PathValue("petID"), input)
	switch {
	case errors.Is(err, expenses.ErrInvalid):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid budget")
	case errors.Is(err, expenses.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, budget)
	}
}

func (s *Server) petExpenseSummary(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	summary, err := s.expenses.PetSummary(r.Context(), principal.UserID, r.PathValue("petID"), r.URL.Query().Get("period"))
	switch {
	case errors.Is(err, expenses.ErrInvalid):
		writeProblem(w, http.StatusBadRequest, "Invalid pet or period")
	case errors.Is(err, expenses.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, summary)
	}
}

func (s *Server) uploadReceipt(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	if s.cookies.Metrics != nil {
		defer func() { s.cookies.Metrics.ObserveReceipt(time.Since(started)) }()
	}
	r.Body = http.MaxBytesReader(w, r.Body, receipts.MaxUploadBytes+1024*1024)
	if err := r.ParseMultipartForm(1024 * 1024); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid multipart upload")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Receipt file is required")
		return
	}
	defer file.Close()
	metadata, err := receiptMetadata(r)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid receipt metadata")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	receipt, err := s.receipts.Upload(r.Context(), r.PathValue("expenseID"), principal.UserID, file, header.Filename, metadata)
	switch {
	case errors.Is(err, expenses.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Expense not found")
	case errors.Is(err, receipts.ErrTooLarge):
		writeProblem(w, http.StatusRequestEntityTooLarge, "Receipt exceeds 10 MiB")
	case errors.Is(err, receipts.ErrInvalidFile):
		writeProblem(w, http.StatusUnsupportedMediaType, "Only valid JPEG, PNG and PDF files are accepted")
	case errors.Is(err, receipts.ErrInvalid):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid receipt metadata")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusCreated, receipt)
	}
}

// parseInstant accepts an RFC 3339 instant or a plain date, which is read as
// UTC midnight. Both forms appear in the query parameters of the calendar
// range: the web UI sends instants, a curl call by hand usually a date.
func parseInstant(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	if instant, err := time.Parse(time.RFC3339, value); err == nil {
		return instant.UTC(), true
	}
	if day, err := time.Parse(time.DateOnly, value); err == nil {
		return day.UTC(), true
	}
	return time.Time{}, false
}

func (s *Server) listCalendarEvents(w http.ResponseWriter, r *http.Request) {
	from, okFrom := parseInstant(r.URL.Query().Get("from"))
	to, okTo := parseInstant(r.URL.Query().Get("to"))
	if !okFrom || !okTo {
		writeProblem(w, http.StatusBadRequest, "Query parameters from and to are required")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	events, err := s.calendar.List(r.Context(), principal.UserID, calendar.Range{From: from, To: to, PetID: r.URL.Query().Get("petId")})
	switch {
	case errors.Is(err, calendar.ErrRange):
		writeProblem(w, http.StatusBadRequest, "Range must be positive and at most 366 days")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"events": events})
	}
}

func (s *Server) createCalendarEvent(w http.ResponseWriter, r *http.Request) {
	var input calendar.EventInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	event, err := s.calendar.Create(r.Context(), input, principal.UserID)
	switch {
	case errors.Is(err, calendar.ErrInvalid):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid calendar event")
	case err != nil:
		s.internalError(w, err)
	default:
		w.Header().Set("Location", "/api/v1/calendar/events/"+event.ID)
		writeJSON(w, http.StatusCreated, event)
	}
}

func (s *Server) getCalendarEvent(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	event, err := s.calendar.Get(r.Context(), principal.UserID, r.PathValue("eventID"))
	s.writeCalendarEvent(w, event, err)
}

func (s *Server) updateCalendarEvent(w http.ResponseWriter, r *http.Request) {
	var input calendar.EventInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	event, err := s.calendar.Update(r.Context(), principal.UserID, r.PathValue("eventID"), input)
	s.writeCalendarEvent(w, event, err)
}

func (s *Server) deleteCalendarEvent(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	switch err := s.calendar.Delete(r.Context(), principal.UserID, r.PathValue("eventID")); {
	case errors.Is(err, calendar.ErrInvalidID):
		writeProblem(w, http.StatusBadRequest, "Invalid event id")
	case errors.Is(err, calendar.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Calendar event not found")
	case err != nil:
		s.internalError(w, err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// writeCalendarEvent keeps the three outcomes identical for read and update.
// A private event of the other household member answers 404 rather than 403,
// so the response does not disclose that it exists.
func (s *Server) writeCalendarEvent(w http.ResponseWriter, event calendar.Event, err error) {
	switch {
	case errors.Is(err, calendar.ErrInvalidID):
		writeProblem(w, http.StatusBadRequest, "Invalid event id")
	case errors.Is(err, calendar.ErrInvalid):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid calendar event")
	case errors.Is(err, calendar.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Calendar event not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, event)
	}
}

func (s *Server) listReceipts(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	items, err := s.receipts.List(r.Context(), principal.UserID, r.PathValue("expenseID"))
	switch {
	case errors.Is(err, receipts.ErrInvalid):
		writeProblem(w, http.StatusBadRequest, "Invalid expense id")
	case errors.Is(err, expenses.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Expense not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"receipts": items})
	}
}

func (s *Server) listDocuments(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), "100"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid limit")
		return
	}
	items, err := s.documents.List(r.Context(), principal.UserID, r.PathValue("petID"), limit)
	switch {
	case errors.Is(err, documents.ErrInvalid):
		writeProblem(w, http.StatusBadRequest, "Invalid pet or limit")
	case errors.Is(err, documents.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"documents": items})
	}
}

func (s *Server) uploadDocument(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, receipts.MaxUploadBytes+1024*1024)
	if err := r.ParseMultipartForm(receipts.MaxUploadBytes); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid multipart request")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Document file is required")
		return
	}
	defer file.Close()
	metadata := documents.Metadata{Title: r.FormValue("title"), DocumentType: r.FormValue("documentType"), Notes: r.FormValue("notes")}
	if value := r.FormValue("occurredOn"); value != "" {
		metadata.OccurredOn = &value
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	document, err := s.documents.Upload(r.Context(), principal.UserID, r.PathValue("petID"), file, header.Filename, metadata)
	switch {
	case errors.Is(err, documents.ErrInvalid):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid document metadata")
	case errors.Is(err, documents.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Pet not found")
	case errors.Is(err, receipts.ErrTooLarge):
		writeProblem(w, http.StatusRequestEntityTooLarge, "Document exceeds 10 MiB")
	case errors.Is(err, receipts.ErrInvalidFile):
		writeProblem(w, http.StatusUnsupportedMediaType, "Only JPEG, PNG and PDF documents are accepted")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusCreated, document)
	}
}

func (s *Server) downloadDocument(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	document, body, err := s.documents.Content(r.Context(), principal.UserID, r.PathValue("petID"), r.PathValue("documentID"))
	switch {
	case errors.Is(err, documents.ErrInvalid):
		writeProblem(w, http.StatusBadRequest, "Invalid pet or document id")
		return
	case errors.Is(err, documents.ErrNotFound), errors.Is(err, os.ErrNotExist):
		writeProblem(w, http.StatusNotFound, "Document not found")
		return
	case err != nil:
		s.internalError(w, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", pinnedMIMEType(document.MIMEType))
	w.Header().Set("Content-Disposition", attachmentDisposition(document.OriginalFilename))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	http.ServeContent(w, r, "", document.UploadedAt, body)
}

// downloadReceipt serves a stored receipt file. It never renders inline: the
// response is an attachment, the Content-Type is pinned to the type detected
// by magic bytes at upload time, and a per-response CSP forbids everything a
// document could try to load.
func (s *Server) downloadReceipt(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	receipt, body, err := s.receipts.Content(r.Context(), principal.UserID, r.PathValue("expenseID"), r.PathValue("receiptID"))
	switch {
	case errors.Is(err, receipts.ErrInvalid):
		writeProblem(w, http.StatusBadRequest, "Invalid expense or receipt id")
		return
	case errors.Is(err, receipts.ErrNotFound), errors.Is(err, os.ErrNotExist):
		writeProblem(w, http.StatusNotFound, "Receipt not found")
		return
	case err != nil:
		s.internalError(w, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", pinnedMIMEType(receipt.MIMEType))
	w.Header().Set("Content-Disposition", attachmentDisposition(receipt.OriginalFilename))
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	http.ServeContent(w, r, "", receipt.UploadedAt, body)
}

// pinnedMIMEType only echoes types the upload validation can actually produce.
// Anything else - a hand-edited database row, a type added later without
// thinking about the download path - degrades to an opaque byte stream.
func pinnedMIMEType(stored string) string {
	switch stored {
	case "image/jpeg", "image/png", "application/pdf":
		return stored
	default:
		return "application/octet-stream"
	}
}

// attachmentDisposition builds a Content-Disposition header from a filename
// that originally came from an upload. mime.FormatMediaType quotes and encodes
// it; if it refuses the value, the file is served under a neutral name rather
// than with an unescaped header.
func attachmentDisposition(filename string) string {
	const fallback = `attachment; filename="receipt"`
	if filename == "" {
		return fallback
	}
	formatted := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	if formatted == "" {
		return fallback
	}
	return formatted
}

func receiptMetadata(r *http.Request) (receipts.Metadata, error) {
	metadata := receipts.Metadata{Merchant: r.FormValue("merchant"), Currency: r.FormValue("currency"), Category: r.FormValue("category")}
	if value := r.FormValue("receiptDate"); value != "" {
		metadata.ReceiptDate = &value
	}
	if value := r.FormValue("totalAmountCents"); value != "" {
		amount, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return receipts.Metadata{}, err
		}
		metadata.TotalAmountCents = &amount
	}
	return metadata, nil
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var credentials auth.Credentials
	if err := decodeJSON(w, r, &credentials); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	sourceIP := remoteIP(r)
	key := sourceIP + "|" + strings.ToLower(credentials.Username)
	if !s.limiter.Allow(key) {
		if s.cookies.Metrics != nil {
			s.cookies.Metrics.LoginFailed()
		}
		w.Header().Set("Retry-After", "900")
		writeProblem(w, http.StatusTooManyRequests, "Too many login attempts")
		return
	}
	pair, err := s.sessions.Login(r.Context(), credentials, sourceIP)
	if errors.Is(err, auth.ErrUnauthenticated) {
		if s.cookies.Metrics != nil {
			s.cookies.Metrics.LoginFailed()
		}
		writeProblem(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	s.limiter.Reset(key)
	writeJSON(w, http.StatusOK, pair)
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := decodeJSON(w, r, &input); err != nil || input.RefreshToken == "" {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	pair, err := s.sessions.Refresh(r.Context(), input.RefreshToken, remoteIP(r))
	if errors.Is(err, auth.ErrUnauthenticated) {
		writeProblem(w, http.StatusUnauthorized, "Invalid refresh token")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pair)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refreshToken"`
	}
	if err := decodeJSON(w, r, &input); err != nil || input.RefreshToken == "" {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := s.sessions.Logout(r.Context(), input.RefreshToken, remoteIP(r)); err != nil {
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// versionInfo beantwortet, was die Anwendung ist und was in ihr steckt.
//
// Vorher konnte HomeFlow das nicht: das Binary kannte seine Version nicht, und
// es gab keinen Endpunkt dafür. Am 2026-09-02 liefen unter demselben Tag drei
// verschiedene Images, und niemand konnte von innen sehen, welches (HF-0105).
func (s *Server) versionInfo(w http.ResponseWriter, _ *http.Request) {
	antwort := map[string]any{
		"version":  Version,
		"releases": release.History(),
	}
	if aktuell, ok := release.Current(Version); ok {
		antwort["aktuell"] = aktuell
	} else {
		// Kein Eintrag zur laufenden Version heißt: gebaut, aber nicht über die
		// Pipeline ausgeliefert. Das ist ein Befund und keine Lücke.
		antwort["hinweis"] = "Zu dieser Version gibt es keine Release-Notiz. " +
			"Das Binary wurde gebaut, aber nicht über die Deploy-Pipeline ausgeliefert."
	}
	writeJSON(w, http.StatusOK, antwort)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "homeflow"})
}

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	includeCompleted, err := strconv.ParseBool(defaultString(r.URL.Query().Get("includeCompleted"), "false"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid query parameter")
		return
	}
	limit, err := strconv.Atoi(defaultString(r.URL.Query().Get("limit"), "100"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid query parameter")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	items, err := s.shopping.List(r.Context(), principal.UserID, r.URL.Query().Get("listId"), includeCompleted, r.URL.Query().Get("petId"), limit)
	if errors.Is(err, shopping.ErrInvalid) {
		writeProblem(w, http.StatusBadRequest, "Invalid query parameter")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) listShoppingLists(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	lists, err := s.shopping.Lists(r.Context(), principal.UserID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"lists": lists})
}

func (s *Server) createItem(w http.ResponseWriter, r *http.Request) {
	var input shopping.CreateItem
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	principal, _ := auth.PrincipalFromContext(r.Context())
	item, err := s.shopping.Create(r.Context(), input, principal.UserID)
	if errors.Is(err, shopping.ErrInvalid) {
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid shopping item")
		return
	}
	if errors.Is(err, shopping.ErrNotFound) {
		writeProblem(w, http.StatusNotFound, "Shopping list or pet not found")
		return
	}
	if err != nil {
		s.internalError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/shopping/items/"+item.ID)
	if s.cookies.Metrics != nil {
		s.cookies.Metrics.ShoppingItemCreated()
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) completeItem(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	item, err := s.shopping.Complete(r.Context(), r.PathValue("itemID"), principal.UserID)
	switch {
	case errors.Is(err, shopping.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Shopping item not found")
	case errors.Is(err, shopping.ErrAlreadyDone):
		writeProblem(w, http.StatusConflict, "Shopping item already completed")
	case errors.Is(err, shopping.ErrInvalid):
		writeProblem(w, http.StatusBadRequest, "Invalid shopping item ID")
	case err != nil:
		s.internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, item)
	}
}

func (s *Server) internalError(w http.ResponseWriter, err error) {
	s.logger.Error("request failed", "error", err)
	writeProblem(w, http.StatusInternalServerError, "Internal server error")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, status int, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "about:blank", "title": title, "status": status})
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func requestLogger(logger *slog.Logger, metrics *observability.Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		if metrics != nil {
			metrics.ObserveHTTP(r.Pattern, r.Method, strconv.Itoa(wrapped.status), time.Since(started))
		}
		logger.Info("http request", "method", r.Method, "path", r.URL.Path,
			"status", wrapped.status, "duration_ms", time.Since(started).Milliseconds())
	})
}

func recoverer(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic recovered", "value", recovered, "path", r.URL.Path)
				writeProblem(w, http.StatusInternalServerError, "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func BearerToken(header string) string {
	return strings.TrimPrefix(header, "Bearer ")
}
