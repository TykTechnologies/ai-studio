import React from "react";
import { ContentContainer, Logo, FormWrapper } from "../styles/authStyles";
import { Box } from "@mui/material";
import backgroundImage from "./login_background.png";
import logoImage from "./login_logo.png";

const AuthLayout = ({ children }) => {
  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        justifyContent: "center",
        minHeight: "100vh",
        backgroundImage: `url(${backgroundImage})`,
        backgroundSize: "cover",
        backgroundPosition: "center",
        padding: 0,
      }}
    >
      <ContentContainer>
        <Logo src={logoImage} alt="Logo" />
        <FormWrapper>
          {children}
        </FormWrapper>
      </ContentContainer>
    </Box>
  );
};

export default AuthLayout;