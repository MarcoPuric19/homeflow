export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message); }
}

function cookie(name: string): string {
  const prefix = `${encodeURIComponent(name)}=`;
  return document.cookie.split("; ").find((part) => part.startsWith(prefix))?.slice(prefix.length) ?? "";
}

async function request<T>(path: string, init: RequestInit = {}, retry = true): Promise<T> {
  const headers = new Headers(init.headers);
  if (!(init.body instanceof FormData) && init.body) headers.set("Content-Type", "application/json");
  if (init.method && !["GET", "HEAD"].includes(init.method)) headers.set("X-CSRF-Token", decodeURIComponent(cookie("homeflow_csrf")));
  const response = await fetch(path, { ...init, headers, credentials: "same-origin" });
  if (response.status === 401 && retry && !path.includes("/auth/browser/")) {
    const refreshed = await fetch("/api/v1/auth/browser/refresh", { method: "POST", credentials: "same-origin" });
    if (refreshed.ok) return request<T>(path, init, false);
  }
  if (!response.ok) {
    const problem = await response.json().catch(() => ({ title: "Unbekannter Fehler" }));
    throw new ApiError(response.status, problem.title ?? "Anfrage fehlgeschlagen");
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

export const api = {
  login: (username: string, password: string) => request<{ username: string }>("/api/v1/auth/browser/login", { method: "POST", body: JSON.stringify({ username, password }) }, false),
  logout: () => request<void>("/api/v1/auth/browser/logout", { method: "POST" }, false),
  shopping: () => request<{ items: ShoppingItem[] }>("/api/v1/shopping/items?includeCompleted=true"),
  shoppingLists: () => request<{ lists: ShoppingList[] }>("/api/v1/shopping/lists"),
  addShopping: (input: ShoppingInput) => request<ShoppingItem>("/api/v1/shopping/items", { method: "POST", body: JSON.stringify(input) }),
  completeShopping: (id: string) => request<ShoppingItem>(`/api/v1/shopping/items/${id}/complete`, { method: "POST" }),
  expenses: (month: string, petId = "") => request<{ expenses: Expense[] }>(`/api/v1/expenses?month=${encodeURIComponent(month)}${petId ? `&petId=${encodeURIComponent(petId)}` : ""}&limit=100`),
  summary: (month: string) => request<ExpenseSummary>(`/api/v1/expense-summaries/${month}`),
  addExpense: (input: ExpenseInput) => request<Expense>("/api/v1/expenses", { method: "POST", body: JSON.stringify(input) }),
  payExpense: (id: string, amount: number) => request<Expense>(`/api/v1/expenses/${id}/paid`, { method: "POST", body: JSON.stringify({ actualAmountCents: amount, paidAt: new Date().toISOString() }) }),
  uploadReceipt: (id: string, data: FormData) => request<unknown>(`/api/v1/expenses/${id}/receipts`, { method: "POST", body: data }),
  receipts: (expenseId: string) => request<{ receipts: Receipt[] }>(`/api/v1/expenses/${expenseId}/receipts`),
  petBudgets: (petId: string, month: string) => request<{ budgets: PetBudget[] }>(`/api/v1/pets/${encodeURIComponent(petId)}/budgets?month=${encodeURIComponent(month)}`),
  savePetBudget: (petId: string, input: PetBudgetInput) => request<PetBudget>(`/api/v1/pets/${encodeURIComponent(petId)}/budgets`, { method: "PUT", body: JSON.stringify(input) }),
  petExpenseSummary: (petId: string, period: string) => request<PetExpenseSummary>(`/api/v1/pets/${encodeURIComponent(petId)}/expense-summary?period=${encodeURIComponent(period)}`),
  calendarEvents: (range: { from: string; to: string }) =>
    request<{ events: CalendarEvent[] }>(`/api/v1/calendar/events?from=${encodeURIComponent(range.from)}&to=${encodeURIComponent(range.to)}`),
  addCalendarEvent: (input: CalendarEventInput) => request<CalendarEvent>("/api/v1/calendar/events", { method: "POST", body: JSON.stringify(input) }),
  updateCalendarEvent: (id: string, input: CalendarEventInput) => request<CalendarEvent>(`/api/v1/calendar/events/${id}`, { method: "PUT", body: JSON.stringify(input) }),
  deleteCalendarEvent: (id: string) => request<void>(`/api/v1/calendar/events/${id}`, { method: "DELETE" }),
  pets: () => request<PetPage>("/api/v1/pets?includeInactive=true&limit=100"),
  addPet: (input: PetInput) => request<Pet>("/api/v1/pets", { method: "POST", body: JSON.stringify(input) }),
  updatePet: (id: string, input: PetInput) => request<Pet>(`/api/v1/pets/${id}`, { method: "PATCH", body: JSON.stringify(input) }),
  medicalRecords: (petId: string) => request<MedicalRecordPage>(`/api/v1/pets/${petId}/medical-records?limit=100`),
  addMedicalRecord: (petId: string, input: MedicalRecordInput) => request<MedicalRecord>(`/api/v1/pets/${petId}/medical-records`, { method: "POST", body: JSON.stringify(input) }),
  vaccinations: (petId: string) => request<VaccinationPage>(`/api/v1/pets/${petId}/vaccinations?limit=100`),
  addVaccination: (petId: string, input: VaccinationInput) => request<Vaccination>(`/api/v1/pets/${petId}/vaccinations`, { method: "POST", body: JSON.stringify(input) }),
  petMedications: (petId: string) => request<PetMedicationPage>(`/api/v1/pets/${petId}/medications?limit=100`),
  addPetMedication: (petId: string, input: PetMedicationInput) => request<PetMedication>(`/api/v1/pets/${petId}/medications`, { method: "POST", body: JSON.stringify(input) }),
  petDocuments: (petId: string) => request<{ documents: PetDocument[] }>(`/api/v1/pets/${encodeURIComponent(petId)}/documents?limit=100`),
  uploadPetDocument: (petId: string, data: FormData) => request<PetDocument>(`/api/v1/pets/${encodeURIComponent(petId)}/documents`, { method: "POST", body: data }),
  petWeights: (petId:string) => request<{weights:WeightEntry[]}>(`/api/v1/pets/${encodeURIComponent(petId)}/weights?limit=100`),
  addPetWeight: (petId:string,input:WeightInput) => request<WeightEntry>(`/api/v1/pets/${encodeURIComponent(petId)}/weights`,{method:"POST",body:JSON.stringify(input)}),
  petLabResults: (petId:string) => request<{labResults:LabResult[]}>(`/api/v1/pets/${encodeURIComponent(petId)}/lab-results?limit=100`),
  addPetLabResult: (petId:string,input:LabResultInput) => request<LabResult>(`/api/v1/pets/${encodeURIComponent(petId)}/lab-results`,{method:"POST",body:JSON.stringify(input)}),
  reminders: (until:string) => request<{reminders:Reminder[]}>(`/api/v1/reminders?until=${encodeURIComponent(until)}&limit=100`),
  petDashboard: (petId:string,month:string) => request<PetDashboard>(`/api/v1/pets/${encodeURIComponent(petId)}/dashboard?month=${encodeURIComponent(month)}`),
  documentLinks: (petId:string,documentId:string) => request<RelationshipSet>(`/api/v1/pets/${encodeURIComponent(petId)}/documents/${encodeURIComponent(documentId)}/links`),
  saveDocumentLinks: (petId:string,documentId:string,input:RelationshipSet) => request<RelationshipSet>(`/api/v1/pets/${encodeURIComponent(petId)}/documents/${encodeURIComponent(documentId)}/links`,{method:"PUT",body:JSON.stringify(input)}),
  expenseLinks: (expenseId:string) => request<RelationshipSet>(`/api/v1/expenses/${encodeURIComponent(expenseId)}/pet-links`),
  saveExpenseLinks: (expenseId:string,input:RelationshipSet) => request<RelationshipSet>(`/api/v1/expenses/${encodeURIComponent(expenseId)}/pet-links`,{method:"PUT",body:JSON.stringify(input)}),
};

// The download is a plain link, not a fetch: the API answers with
// Content-Disposition: attachment, so the browser stores the file instead of
// rendering it. Ids are encoded because they end up in a URL path.
export const receiptContentPath = (expenseId: string, receiptId: string) =>
  `/api/v1/expenses/${encodeURIComponent(expenseId)}/receipts/${encodeURIComponent(receiptId)}/content`;
export const petDocumentContentPath = (petId: string, documentId: string) =>
  `/api/v1/pets/${encodeURIComponent(petId)}/documents/${encodeURIComponent(documentId)}/content`;

export type ShoppingList = { id:string;name:string;visibility:"shared"|"private" };
export type ShoppingItem = { id: string; name: string; quantity: string; unit?: string; category?: string; priority: string; completed: boolean; petIds:string[] };
export type ShoppingInput = { listId: string; name: string; quantity: string; unit: string; category: string; note: string; priority: string; store: string; petIds:string[] };
export type Expense = { id: string; description: string; merchant?: string; plannedAmountCents: number; actualAmountCents?: number; currency: string; dueDate: string; category?: string; status: string; petId?: string; budgetCategory?: string };
export type ExpenseInput = { description: string; merchant: string; plannedAmountCents: number; currency: string; dueDate: string; category: string; recurrenceRule: string; status: string; paymentMethod: string; notes: string; petId?: string; budgetCategory?: string };
export type Receipt = { id: string; expenseId: string; originalFilename: string; mimeType: string; sha256: string; uploadedAt: string; merchant?: string; totalAmountCents?: number; currency: string };
export type AppointmentType = "veterinarian" | "vaccination" | "medication" | "grooming" | "other";
export type CalendarEvent = { id: string; title: string; description?: string; startsAt: string; endsAt: string; allDay: boolean; category?: string; ownerId: string; visibility: "shared" | "private"; recurrenceRule?: string; reminderMinutes?: number; status: "planned" | "completed" | "cancelled"; petId?: string; appointmentType?: AppointmentType };
export type CalendarEventInput = { title: string; description?: string; startsAt: string; endsAt: string; allDay?: boolean; category?: string; visibility?: "shared" | "private"; recurrenceRule?: string; reminderMinutes?: number; status?: "planned" | "completed" | "cancelled"; petId?: string; appointmentType?: AppointmentType };
export type ExpenseSummary = { month: string; plannedAmountCents: number; actualAmountCents: number; openAmountCents: number; planActualDeltaCents: number };
export type PetBudgetInput = { month:string;category:string;amountCents:number;currency:string };
export type PetBudget = PetBudgetInput & { id:string;petId:string;updatedAt:string };
export type PetExpenseCategorySummary = { category:string;plannedAmountCents:number;actualAmountCents:number;budgetAmountCents:number };
export type PetExpenseSummary = { petId:string;period:string;categories:PetExpenseCategorySummary[] };
export type Pet = { id: string; name: string; species: string; breed?: string; birthDate?: string; approximateBirthDate: boolean; sex: "female" | "male" | "unknown"; microchipNumber?: string; color?: string; insurance?: string; insuranceNumber?: string; veterinarian?: string; notes?: string; active: boolean; createdAt: string; updatedAt: string };
export type PetInput = { name: string; species: string; breed?: string; birthDate?: string; approximateBirthDate?: boolean; sex?: "female" | "male" | "unknown"; microchipNumber?: string; color?: string; insurance?: string; insuranceNumber?: string; veterinarian?: string; notes?: string; active?: boolean };
export type PetPage = { pets: Pet[]; nextCursor?: string };
export type MedicalRecord = { id: string; petId: string; recordType: MedicalRecordType; occurredOn: string; title: string; summary?: string; veterinarian?: string; details: Record<string, string>; createdAt: string; updatedAt: string };
export type MedicalRecordType = "diagnosis" | "treatment" | "procedure" | "examination" | "note" | "imaging" | "report";
export type MedicalRecordInput = { recordType: MedicalRecordType; occurredOn: string; title: string; summary?: string; veterinarian?: string; details?: Record<string, string> };
export type MedicalRecordPage = { records: MedicalRecord[]; nextCursor?: string };
export type Vaccination = { id: string; petId: string; vaccineName: string; manufacturer?: string; diseases: string[]; administeredOn: string; validUntil?: string; nextDueDate?: string; documentId?: string; veterinarian?: string; batchNumber?: string; notes?: string; createdAt: string; updatedAt: string };
export type VaccinationInput = { vaccineName: string; manufacturer?: string; diseases?: string[]; administeredOn: string; validUntil?: string; nextDueDate?: string; documentId?: string; veterinarian?: string; batchNumber?: string; notes?: string };
export type VaccinationPage = { vaccinations: Vaccination[]; nextCursor?: string };
export type PetMedication = { id:string;petId:string;medicationId:string;name:string;activeIngredient?:string;dosageForm?:string;doseText:string;frequencyText:string;startsOn:string;endsOn?:string;stockQuantity:string;stockUnit?:string;notes?:string;active:boolean;createdAt:string;updatedAt:string };
export type PetMedicationInput = { name:string;activeIngredient?:string;dosageForm?:string;doseText:string;frequencyText:string;startsOn:string;endsOn?:string;stockQuantity:string;stockUnit?:string;notes?:string;active?:boolean };
export type PetMedicationPage = { medications:PetMedication[] };
export type PetDocument = { id:string;petId:string;title:string;documentType:"report"|"lab"|"vaccination"|"imaging"|"insurance"|"other";occurredOn?:string;notes?:string;originalFilename:string;mimeType:string;sha256:string;uploadedBy:string;uploadedAt:string };
export type WeightEntry={id:string;petId:string;measuredOn:string;weightKg:string;notes?:string;createdAt:string};
export type WeightInput={measuredOn:string;weightKg:string;notes:string};
export type LabResult={id:string;petId:string;measuredOn:string;testName:string;value:string;unit?:string;referenceLow?:string;referenceHigh?:string;notes?:string;createdAt:string};
export type LabResultInput={measuredOn:string;testName:string;value:string;unit:string;referenceLow?:string;referenceHigh?:string;notes:string};
export type Reminder={id:string;kind:"vaccination"|"appointment";petId:string;title:string;dueDate:string;daysUntil:number};
export type RelationshipSet={medicalRecordIds:string[];appointmentIds:string[]};
export type PetDashboard={petId:string;petName:string;medicalRecordCount:number;documentCount:number;activeMedicationCount:number;openShoppingItemCount:number;latestWeightKg?:string;latestWeightDate?:string;nextVaccinationName?:string;nextVaccinationDate?:string;nextAppointmentTitle?:string;nextAppointmentAt?:string;month:string;budgetAmountCents:number;plannedAmountCents:number;actualAmountCents:number};
