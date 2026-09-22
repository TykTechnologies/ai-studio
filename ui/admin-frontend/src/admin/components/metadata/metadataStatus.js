/**
 * The governed-metadata status vocabulary, shared by the Metadata coverage
 * page and the status chip on detail pages so both show the same label and
 * colour for a status.
 */
export const STATUSES = [
  { value: "missing", label: "Missing", color: "error" },
  { value: "invalid", label: "Invalid", color: "error" },
  { value: "expired", label: "Expired", color: "warning" },
  { value: "warnings", label: "Warnings", color: "warning" },
  { value: "valid", label: "Valid", color: "success" },
];

export const statusMeta = (status) =>
  STATUSES.find((s) => s.value === status) || { value: status, label: status, color: "default" };
