import { describe, expect, it } from "vitest";
import { petDocumentContentPath, receiptContentPath } from "./api";

describe("receiptContentPath", () => {
  it("builds the download path for a receipt", () => {
    expect(receiptContentPath("e1", "r1")).toBe("/api/v1/expenses/e1/receipts/r1/content");
  });

  it("encodes ids so they cannot escape their path segment", () => {
    const path = receiptContentPath("../../etc", "a b/c");
    expect(path).toBe("/api/v1/expenses/..%2F..%2Fetc/receipts/a%20b%2Fc/content");
    expect(path.split("/")).toHaveLength(8);
  });
});

describe("petDocumentContentPath", () => {
  it("encodes both identifiers", () => {
    expect(petDocumentContentPath("pet/1", "doc 1")).toBe("/api/v1/pets/pet%2F1/documents/doc%201/content");
  });
});
