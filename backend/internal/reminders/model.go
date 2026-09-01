package reminders

import "time"

type Reminder struct {
	ID        string `json:"id"`
	SourceID  string `json:"sourceId"`
	Kind      string `json:"kind"`
	PetID     string `json:"petId"`
	Title     string `json:"title"`
	DueDate   string `json:"dueDate"`
	DaysUntil int    `json:"daysUntil"`
	LeadDays  int    `json:"leadDays"`
}

type AppointmentInput struct {
	Title      string    `json:"title"`
	StartsAt   time.Time `json:"startsAt"`
	EndsAt     time.Time `json:"endsAt"`
	Visibility string    `json:"visibility"`
}

type Rules struct {
	LeadDays []int `json:"leadDays"`
}
