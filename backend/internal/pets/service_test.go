package pets

import (
	"context"
	"testing"
	"time"
)

type stubRepository struct {
	created Pet
	page    Page
	gotUser string
}

func (r *stubRepository) Create(_ context.Context, userID, _ string, pet Pet) (Pet, error) {
	r.gotUser, r.created = userID, pet
	return pet, nil
}
func (r *stubRepository) List(_ context.Context, userID string, _ bool, _ int, _ string) (Page, error) {
	r.gotUser = userID
	return r.page, nil
}
func (r *stubRepository) Get(_ context.Context, userID, _ string) (Pet, error) {
	r.gotUser = userID
	return Pet{}, nil
}
func (r *stubRepository) Update(_ context.Context, userID string, pet Pet) (Pet, error) {
	r.gotUser = userID
	return pet, nil
}
func (r *stubRepository) CreateWeight(_ context.Context, _, _ string, e WeightEntry) (WeightEntry, error) {
	return e, nil
}
func (r *stubRepository) ListWeights(context.Context, string, string, int) ([]WeightEntry, error) {
	return []WeightEntry{}, nil
}
func (r *stubRepository) CreateLabResult(_ context.Context, _, _ string, e LabResult) (LabResult, error) {
	return e, nil
}
func (r *stubRepository) ListLabResults(context.Context, string, string, int) ([]LabResult, error) {
	return []LabResult{}, nil
}
func (r *stubRepository) Dashboard(_ context.Context, _ string, petID, month string, _ time.Time) (Dashboard, error) {
	return Dashboard{PetID: petID, Month: month}, nil
}

func TestCreateNormalisesAndScopesPet(t *testing.T) {
	repository := &stubRepository{}
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) }
	pet, err := service.Create(context.Background(), "00000000-0000-4000-8000-000000000011", Input{Name: " Momo ", Species: " Katze "})
	if err != nil {
		t.Fatal(err)
	}
	if pet.Name != "Momo" || pet.Species != "Katze" || pet.Sex != SexUnknown || !pet.Active {
		t.Fatalf("unexpected pet: %#v", pet)
	}
	if repository.gotUser == "" {
		t.Fatal("user scope was not passed to repository")
	}
}

func TestPetProfileDetailsAreOptionalAndNormalised(t *testing.T) {
	repository := &stubRepository{}
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) }
	birthDate := "2021-04-01"
	pet, err := service.Create(context.Background(), "user", Input{
		Name: "Momo", Species: "Katze", BirthDate: &birthDate, ApproximateBirthDate: true,
		Color: " Getigert ", Insurance: " TierSchutz ", InsuranceNumber: " TS-42 ", Veterinarian: " Praxis am Park ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !pet.ApproximateBirthDate || pet.Color != "Getigert" || pet.Insurance != "TierSchutz" || pet.InsuranceNumber != "TS-42" || pet.Veterinarian != "Praxis am Park" {
		t.Fatalf("unexpected pet details: %#v", pet)
	}
	if _, err := service.Create(context.Background(), "user", Input{Name: "Momo", Species: "Katze", ApproximateBirthDate: true}); err != ErrInvalid {
		t.Fatalf("approximate date without birth date err=%v", err)
	}
}

func TestListRejectsUnboundedAndMalformedCursor(t *testing.T) {
	service := NewService(&stubRepository{})
	if _, err := service.List(context.Background(), "user", false, MaxLimit+1, ""); err != ErrInvalid {
		t.Fatalf("got %v", err)
	}
	if _, err := service.List(context.Background(), "user", false, 10, "not-a-uuid"); err != ErrInvalid {
		t.Fatalf("got %v", err)
	}
}

func TestUpdateRejectsFutureBirthDate(t *testing.T) {
	service := NewService(&stubRepository{})
	service.now = func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) }
	future := "2026-09-02"
	active := true
	_, err := service.Update(context.Background(), "user", "00000000-0000-4000-8000-000000000012", Input{Name: "Momo", Species: "Katze", BirthDate: &future, Active: &active})
	if err != ErrInvalid {
		t.Fatalf("got %v", err)
	}
}

func TestMedicalRecordValidationAndCursor(t *testing.T) {
	repository := NewMemoryRepository()
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) }
	pet, err := service.Create(context.Background(), "user", Input{Name: "Momo", Species: "Katze"})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.CreateMedicalRecord(context.Background(), "user", pet.ID, MedicalRecordInput{RecordType: RecordExamination, OccurredOn: "2026-08-30", Title: "Kontrolle"})
	if err != nil || record.PetID != pet.ID {
		t.Fatalf("record=%#v err=%v", record, err)
	}
	page, err := service.ListMedicalRecords(context.Background(), "user", pet.ID, 1, "")
	if err != nil || len(page.Records) != 1 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	_, err = service.CreateMedicalRecord(context.Background(), "user", pet.ID, MedicalRecordInput{RecordType: "advice", OccurredOn: "2026-08-30", Title: "Nicht erlaubt"})
	if err != ErrInvalidRecord {
		t.Fatalf("got %v", err)
	}
}

func TestVaccinationUsesOnlyManualDueDate(t *testing.T) {
	repository := NewMemoryRepository()
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) }
	pet, err := service.Create(context.Background(), "user", Input{Name: "Momo", Species: "Katze"})
	if err != nil {
		t.Fatal(err)
	}
	due := "2027-08-30"
	validUntil := "2027-08-30"
	vaccination, err := service.CreateVaccination(context.Background(), "user", pet.ID, VaccinationInput{VaccineName: "Tollwut", Manufacturer: "Beispiel", Diseases: []string{"Tollwut"}, AdministeredOn: "2026-08-30", ValidUntil: &validUntil, NextDueDate: &due})
	if err != nil || vaccination.NextDueDate == nil || *vaccination.NextDueDate != due || vaccination.Manufacturer != "Beispiel" || len(vaccination.Diseases) != 1 {
		t.Fatalf("vaccination=%#v err=%v", vaccination, err)
	}
	before := "2026-08-29"
	if _, err := service.CreateVaccination(context.Background(), "user", pet.ID, VaccinationInput{VaccineName: "Fehler", AdministeredOn: "2026-08-30", NextDueDate: &before}); err != ErrInvalidVaccination {
		t.Fatalf("got %v", err)
	}
	if _, err := service.CreateVaccination(context.Background(), "user", pet.ID, VaccinationInput{VaccineName: "Fehler", Diseases: []string{"A", "A"}, AdministeredOn: "2026-08-30"}); err != ErrInvalidVaccination {
		t.Fatalf("duplicate diseases err=%v", err)
	}
}

func TestPetMedicationKeepsDocumentedDoseAndExactStock(t *testing.T) {
	repository := NewMemoryRepository()
	service := NewService(repository)
	service.now = func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) }
	pet, err := service.Create(context.Background(), "user", Input{Name: "Momo", Species: "Katze"})
	if err != nil {
		t.Fatal(err)
	}
	medication, err := service.CreatePetMedication(context.Background(), "user", pet.ID, PetMedicationInput{Name: "Mittel", DoseText: "laut Tierarzt", FrequencyText: "morgens", StartsOn: "2026-09-01", StockQuantity: "12.500", StockUnit: "Tabletten"})
	if err != nil || medication.StockQuantity != "12.500" {
		t.Fatalf("medication=%#v err=%v", medication, err)
	}
	if _, err := service.CreatePetMedication(context.Background(), "user", pet.ID, PetMedicationInput{Name: "Mittel", DoseText: "x", FrequencyText: "x", StartsOn: "2026-09-01", StockQuantity: "1.2345"}); err != ErrInvalidMedication {
		t.Fatalf("got %v", err)
	}
}

func TestCreateMeasurementsPreservesExactDecimals(t *testing.T) {
	service := NewService(&stubRepository{})
	service.now = func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) }
	petID := "00000000-0000-4000-8000-000000000010"
	weight, err := service.CreateWeight(context.Background(), "user", petID, WeightInput{MeasuredOn: "2026-09-01", WeightKg: "4.250"})
	if err != nil || weight.WeightKg != "4.250" {
		t.Fatalf("weight=%#v err=%v", weight, err)
	}
	low, high := "4.000", "8.000"
	lab, err := service.CreateLabResult(context.Background(), "user", petID, LabResultInput{MeasuredOn: "2026-09-01", TestName: "Glukose", Value: "5.5000", Unit: "mmol/L", ReferenceLow: &low, ReferenceHigh: &high})
	if err != nil || lab.Value != "5.5000" {
		t.Fatalf("lab=%#v err=%v", lab, err)
	}
	badLow, badHigh := "9", "2"
	if _, err := service.CreateLabResult(context.Background(), "user", petID, LabResultInput{MeasuredOn: "2026-09-01", TestName: "Fehler", Value: "5", ReferenceLow: &badLow, ReferenceHigh: &badHigh}); err != ErrInvalidMeasurement {
		t.Fatalf("reversed range err=%v", err)
	}
}

func TestDashboardValidatesBoundedMonth(t *testing.T) {
	service := NewService(&stubRepository{})
	petID := "00000000-0000-4000-8000-000000000010"
	dashboard, err := service.Dashboard(context.Background(), "user", petID, "2026-09")
	if err != nil || dashboard.Month != "2026-09" {
		t.Fatalf("dashboard=%#v err=%v", dashboard, err)
	}
	if _, err := service.Dashboard(context.Background(), "user", petID, "all"); err != ErrInvalid {
		t.Fatalf("err=%v", err)
	}
}
