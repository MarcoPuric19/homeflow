package pets

import "time"

type Dashboard struct {
	PetID                 string     `json:"petId"`
	PetName               string     `json:"petName"`
	MedicalRecordCount    int        `json:"medicalRecordCount"`
	DocumentCount         int        `json:"documentCount"`
	ActiveMedicationCount int        `json:"activeMedicationCount"`
	OpenShoppingItemCount int        `json:"openShoppingItemCount"`
	LatestWeightKg        *string    `json:"latestWeightKg,omitempty"`
	LatestWeightDate      *string    `json:"latestWeightDate,omitempty"`
	NextVaccinationName   *string    `json:"nextVaccinationName,omitempty"`
	NextVaccinationDate   *string    `json:"nextVaccinationDate,omitempty"`
	NextAppointmentTitle  *string    `json:"nextAppointmentTitle,omitempty"`
	NextAppointmentAt     *time.Time `json:"nextAppointmentAt,omitempty"`
	Month                 string     `json:"month"`
	BudgetAmountCents     int64      `json:"budgetAmountCents"`
	PlannedAmountCents    int64      `json:"plannedAmountCents"`
	ActualAmountCents     int64      `json:"actualAmountCents"`
}
