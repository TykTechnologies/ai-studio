import React from "react";
import { Chip } from "@mui/material";

const COLORS = { proposed: "warning", approved: "success", retired: "default" };

/** PromptTemplateStatus is a template version's status as a chip. */
const PromptTemplateStatus = ({ status }) => (
  <Chip size="small" label={status} color={COLORS[status] || "default"} variant="outlined" />
);

export default PromptTemplateStatus;
