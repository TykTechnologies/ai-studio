import { memo, useEffect } from "react";
import { Typography, Box } from "@mui/material";
import { StyledTextField } from "../../../styles/sharedStyles";
import { StyledCheckbox } from "../../../../portal/styles/authStyles";
import CollapsibleSection from "../../common/CollapsibleSection";
import InfoTooltip from "../../common/InfoTooltip";
import { useFormValidation } from "../hooks/useFormValidation";
import { useParams } from "react-router-dom";

const UserFormBasicInfo = memo(({
  name,
  setName,
  email,
  setEmail,
  password,
  setPassword,
  emailVerified,
  setEmailVerified,
  setBasicInfoValid
}) => {
  const { id } = useParams();
  
  const {
    error: emailError,
    handleChange: handleEmailChange
  } = useFormValidation(email, false, true);

  const {
    error: passwordError,
    handleChange: handlePasswordChange
  } = useFormValidation(password, true, false);

  useEffect(() => {
    const isValid = name.trim() !== "" && 
                   email.trim() !== "" && 
                   (id ? true : password.trim() !== "") &&
                   !emailError && 
                   !passwordError;
    
    setBasicInfoValid(isValid);
  }, [name, email, password, setBasicInfoValid, id, emailError, passwordError]);

  const onEmailChange = (e) => {
    setEmail(e.target.value);
    handleEmailChange(e);
  };

  const onPasswordChange = (e) => {
    setPassword(e.target.value);
    handlePasswordChange(e);
  };

  return (
    <CollapsibleSection title="Basic information*" defaultExpanded={true}>
        <Box>
            <Typography variant="bodyLargeBold" color="text.primary" mb={1}>
                Name*
            </Typography>
            <StyledTextField
              fullWidth
              name="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              autoComplete="off"
            />
        </Box>
        <Box my={2} flexDirection={{ xs: "column", sm: "row" }} display="flex" gap={2}>
            <Box width={{ xs: "100%", sm: "50%" }}>
                <Typography variant="bodyLargeBold" color="text.primary" mb={1}>
                    Email*
                </Typography>
                <StyledTextField
                  fullWidth
                  name="email"
                  type="email"
                  value={email}
                  onChange={onEmailChange}
                  error={!!emailError}
                  helperText={emailError}
                  required
                  autoComplete="new-email"
                  inputProps={{
                      autoComplete: "new-email",
                      "data-form-type": "other"
                  }}
                />
            </Box>
            <Box width={{ xs: "100%", sm: "50%" }}>
                <Box display="flex" alignItems="center">
                    <Typography variant="bodyLargeBold" color="text.primary" mb={0.2} mr={1}>
                        Password*
                    </Typography>
                    <InfoTooltip 
                        title={
                            <Box>
                                <Typography variant="bodyMediumSemiBold">Password requirements</Typography>
                                <Box display="flex" flexDirection="column" p={0.5}>
                                    <Typography variant="bodySmallDefault">• At least 8 characters</Typography>
                                    <Typography variant="bodySmallDefault">• A number</Typography>
                                    <Typography variant="bodySmallDefault">• A special character</Typography>
                                    <Typography variant="bodySmallDefault">• An uppercase letter</Typography>
                                    <Typography variant="bodySmallDefault">• A lowercase letter</Typography>
                                </Box>
                            </Box>
                        }
                    />
                </Box>
                <StyledTextField
                  fullWidth
                  name="password"
                  type="password"
                  value={password}
                  onChange={onPasswordChange}
                  error={!!passwordError}
                  helperText={passwordError}
                  autoComplete="new-password"
                  inputProps={{
                      autoComplete: "new-password",
                      "data-form-type": "other"
                  }}
                />
            </Box>
        </Box>
        <Box mt={1}>
          <StyledCheckbox
              checked={emailVerified}
              onChange={setEmailVerified}
              label="Email address verified"
          />
        </Box>
    </CollapsibleSection>
  );
});

export default UserFormBasicInfo;