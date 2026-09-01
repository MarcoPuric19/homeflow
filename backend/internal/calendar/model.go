package calendar

import "time"

type Visibility string

const (
	VisibilityShared  Visibility = "shared"
	VisibilityPrivate Visibility = "private"
)

type Status string
type AppointmentType string

const (
	StatusPlanned   Status = "planned"
	StatusCompleted Status = "completed"
	StatusCancelled Status = "cancelled"
)

const (
	AppointmentVeterinarian AppointmentType = "veterinarian"
	AppointmentVaccination  AppointmentType = "vaccination"
	AppointmentMedication   AppointmentType = "medication"
	AppointmentGrooming     AppointmentType = "grooming"
	AppointmentOther        AppointmentType = "other"
)

// Event mirrors the calendar_events table. RecurrenceRule holds an RFC 5545
// RRULE verbatim: the rule is stored, never materialised into a series, as
// docs/architecture/domain-model.md requires. ReminderMinutes is stored for the
// same reason - there is no scheduler that could deliver a reminder yet.
type Event struct {
	ID              string     `json:"id"`
	Title           string     `json:"title"`
	Description     string     `json:"description,omitempty"`
	StartsAt        time.Time  `json:"startsAt"`
	EndsAt          time.Time  `json:"endsAt"`
	AllDay          bool       `json:"allDay"`
	Category        string     `json:"category,omitempty"`
	OwnerID         string     `json:"ownerId"`
	Visibility      Visibility `json:"visibility"`
	RecurrenceRule  string     `json:"recurrenceRule,omitempty"`
	ReminderMinutes *int       `json:"reminderMinutes,omitempty"`
	Status          Status     `json:"status"`
	// Pflicht seit HF-0116: es gibt keinen Termin mehr ohne Tier.
	PetID           string           `json:"petId"`
	AppointmentType *AppointmentType `json:"appointmentType,omitempty"`
	CreatedAt       time.Time        `json:"createdAt"`
	UpdatedAt       time.Time        `json:"updatedAt"`
}

// EventInput carries the fields a client may set. The owner is taken from the
// authenticated principal and never from the request body.
type EventInput struct {
	Title           string           `json:"title"`
	Description     string           `json:"description"`
	StartsAt        time.Time        `json:"startsAt"`
	EndsAt          time.Time        `json:"endsAt"`
	AllDay          bool             `json:"allDay"`
	Category        string           `json:"category"`
	Visibility      Visibility       `json:"visibility"`
	RecurrenceRule  string           `json:"recurrenceRule"`
	ReminderMinutes *int             `json:"reminderMinutes"`
	Status          Status           `json:"status"`
	PetID           string           `json:"petId"`
	AppointmentType *AppointmentType `json:"appointmentType,omitempty"`
}

// Range is a half-open interval [From, To).
type Range struct {
	From  time.Time
	To    time.Time
	PetID string
}
