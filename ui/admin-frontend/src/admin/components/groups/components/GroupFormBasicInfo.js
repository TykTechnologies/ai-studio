import React from "react";
import { Typography, Box } from "@mui/material";
import { StyledTextField } from "../../../styles/sharedStyles";
import Section from "../../common/Section";

const GroupFormBasicInfo = ({ name, setName, error }) => {
  return (
    <Section>
      <Typography variant="bodyLargeDefault" color="text.defaultSubdued" sx={{ mb: 3 }}>
        Teams help you organize users and easily manage their access to LLM providers, data sources, and tools through catalogs. Linking teams to specific catalogs ensures they access only AI and data relevant to them.
      </Typography>
      <Box sx={{ my: 2 }}>
        <Typography variant="bodyLargeBold" color="text.primary" sx={{ mb: 1 }}>
          Team name*
        </Typography>
        <StyledTextField
          fullWidth
          name="name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          error={!!error}
          helperText={error}
          required
          autoComplete="off"
        />
      </Box>
    </Section>
  );
};

export default GroupFormBasicInfo;