/**
 * Ready-made client tools. Picking one fills the whole form (name, when the
 * assistant should use it, card text and fields); everything stays
 * editable afterwards.
 *
 * `parameters` are the fields the assistant supplies when it calls the
 * tool (shown to the person as context). `response` are the fields the
 * person fills in (form kind only).
 */
const f = (name, label, type = "text", extra = {}) => ({ name, label, type, required: false, description: "", options: [], ...extra });

export const CLIENT_TOOL_PRESETS = [
  {
    id: "shipping-us",
    kind: "form",
    name: "Shipping address (US)",
    description:
      "Collect a US shipping address from the user. Use this whenever an order, delivery or return needs a destination address in the United States. Do not ask for the address in chat; call this tool instead.",
    title: "Where should we ship to?",
    instructions: "Please fill in the delivery address.",
    parameters: [f("purpose", "Purpose", "text", { required: true, description: "Why the address is needed, e.g. 'to ship order #1234'" })],
    response: [
      f("full_name", "Full name", "text", { required: true }),
      f("address_line1", "Street address", "text", { required: true }),
      f("address_line2", "Apartment, suite, etc.", "text"),
      f("city", "City", "text", { required: true }),
      f("state", "State", "select", {
        required: true,
        options: [
          "AL", "AK", "AZ", "AR", "CA", "CO", "CT", "DE", "DC", "FL", "GA", "HI", "ID", "IL", "IN", "IA", "KS", "KY", "LA", "ME",
          "MD", "MA", "MI", "MN", "MS", "MO", "MT", "NE", "NV", "NH", "NJ", "NM", "NY", "NC", "ND", "OH", "OK", "OR", "PA", "RI",
          "SC", "SD", "TN", "TX", "UT", "VT", "VA", "WA", "WV", "WI", "WY",
        ],
      }),
      f("zip", "ZIP code", "text", { required: true, description: "5 digits, or ZIP+4" }),
      f("phone", "Phone", "phone", { description: "For delivery updates" }),
      f("delivery_notes", "Delivery instructions", "textarea"),
    ],
  },
  {
    id: "shipping-international",
    kind: "form",
    name: "Shipping address (international)",
    description:
      "Collect a shipping address outside the United States. Use this whenever an order, delivery or return needs a destination address and the user is not in the US. Do not ask for the address in chat; call this tool instead.",
    title: "Where should we ship to?",
    instructions: "Please fill in the delivery address, including the country.",
    parameters: [f("purpose", "Purpose", "text", { required: true, description: "Why the address is needed, e.g. 'to ship order #1234'" })],
    response: [
      f("full_name", "Full name", "text", { required: true }),
      f("address_line1", "Address line 1", "text", { required: true }),
      f("address_line2", "Address line 2", "text"),
      f("city", "City / town", "text", { required: true }),
      f("region", "State / province / region", "text"),
      f("postal_code", "Postal code", "text", { required: true }),
      f("country", "Country", "text", { required: true, description: "Full country name or ISO code" }),
      f("phone", "Phone (with country code)", "phone", { description: "Carriers often require it for international parcels" }),
      f("delivery_notes", "Delivery instructions", "textarea"),
    ],
  },
  {
    id: "contact-information",
    kind: "form",
    name: "Contact information",
    description:
      "Collect the user's contact details: telephone, email and social handles. Use this when you need to reach the user later, register them for something, or confirm who they are. Do not ask for these details in chat; call this tool instead.",
    title: "How can we reach you?",
    instructions: "Only the fields you are comfortable sharing are required.",
    parameters: [f("purpose", "Purpose", "text", { required: true, description: "What the details will be used for" })],
    response: [
      f("full_name", "Full name", "text", { required: true }),
      f("email", "Email", "email", { required: true }),
      f("phone", "Telephone", "phone"),
      f("preferred_channel", "Preferred way to contact you", "select", { options: ["Email", "Phone", "Text message", "Social media"] }),
      f("linkedin", "LinkedIn", "url"),
      f("x_handle", "X / Twitter handle", "text"),
      f("other_social", "Other social profile", "url"),
    ],
  },
  {
    id: "confirm-destructive-action",
    kind: "approval",
    name: "Confirm a destructive action",
    description:
      "Ask the user to confirm before deleting, cancelling, overwriting or sending anything on their behalf. Always call this first; only proceed if the user approves.",
    title: "Please confirm",
    instructions: "The assistant is about to do the following.",
    parameters: [
      f("action", "Action", "text", { required: true, description: "What will happen, in one sentence" }),
      f("affected_items", "Affected items", "textarea", { description: "What is affected, e.g. '3 draft documents'" }),
      f("reversible", "Can it be undone?", "boolean"),
    ],
    response: [],
  },
  {
    id: "approve-spend",
    kind: "approval",
    name: "Approve a purchase or spend",
    description:
      "Ask the user to approve spending money: a purchase, a subscription change or a budget increase. Call it before committing any spend; proceed only if approved.",
    title: "Approve this spend?",
    instructions: "Review the amount and what it pays for.",
    parameters: [
      f("amount", "Amount", "number", { required: true, description: "The total to be spent" }),
      f("currency", "Currency", "select", { required: true, options: ["USD", "EUR", "GBP", "NZD", "AUD"] }),
      f("description", "What it pays for", "textarea", { required: true }),
      f("vendor", "Vendor", "text"),
    ],
    response: [],
  },
];

export const presetsForKind = (kind) => CLIENT_TOOL_PRESETS.filter((p) => p.kind === kind);

export default CLIENT_TOOL_PRESETS;
