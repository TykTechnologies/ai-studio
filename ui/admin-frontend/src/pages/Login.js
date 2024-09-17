// src/Login.js
import React, { useState } from "react";
// import { useNavigate } from "react-router-dom";
// import apiClient from "./apiClient";
// import TextField from "@mui/material/TextField";
// import Button from "@mui/material/Button";
import Typography from "@mui/material/Typography";
import Box from "@mui/material/Box";
import Alert from "@mui/material/Alert";

const Login = () => {
  return (
    <Box
      sx={{
        width: "300px",
        margin: "100px auto",
        textAlign: "center",
      }}
    >
      <Typography variant="h5" gutterBottom>
        Login Disabled
      </Typography>
      <Alert severity="info">
        The application is currently running in dev mode. Authentication is
        disabled.
      </Alert>
    </Box>
  );
};

// const Login = () => {
//   const [email, setEmail] = useState("");
//   const [password, setPassword] = useState("");
//   const navigate = useNavigate();
//   const [error, setError] = useState("");

//   const handleSubmit = async (e) => {
//     e.preventDefault();
//     try {
//       // Replace '/auth/login' with your actual login endpoint
//       const response = await apiClient.post("/auth/login", {
//         email,
//         password,
//       });

//       // Assuming the response contains the token in response.data.token
//       localStorage.setItem("token", response.data.token);
//       navigate("/");
//     } catch (err) {
//       setError("Invalid email or password");
//     }
//   };

//   return (
//     <Box
//       sx={{
//         width: "300px",
//         margin: "100px auto",
//       }}
//     >
//       <form onSubmit={handleSubmit}>
//         <h2>Login</h2>
//         {error && (
//           <Alert severity="error" sx={{ mb: 2 }}>
//             {error}
//           </Alert>
//         )}
//         <TextField
//           fullWidth
//           margin="normal"
//           label="Email"
//           type="email"
//           value={email}
//           onChange={(e) => setEmail(e.target.value)}
//           required
//         />
//         <TextField
//           fullWidth
//           margin="normal"
//           label="Password"
//           type="password"
//           value={password}
//           onChange={(e) => setPassword(e.target.value)}
//           required
//         />
//         <Button
//           fullWidth
//           variant="contained"
//           color="primary"
//           type="submit"
//           sx={{ mt: 2 }}
//         >
//           Login
//         </Button>
//       </form>
//     </Box>
//   );
// };

export default Login;
