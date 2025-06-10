import { useState, useEffect } from 'react';

export const useFormValidation = (initialValue = '', isPassword = false, isEmail = false) => {
  const [value, setValue] = useState(initialValue);
  const [error, setError] = useState(null);
  const [passwordCriteria, setPasswordCriteria] = useState({
    length: false,
    number: false,
    special: false,
    uppercase: false,
    lowercase: false,
  });

  useEffect(() => {
    if (isPassword) {
      setPasswordCriteria({
        length: value.length >= 8,
        number: /\d/.test(value),
        special: /[!@#$%^&*(),.?":{}|<>_+=\-~]/.test(value),
        uppercase: /[A-Z]/.test(value),
        lowercase: /[a-z]/.test(value),
      });
    }
  }, [value, isPassword]);

  useEffect(() => {
    setValue(initialValue);
  }, [initialValue]);

  const validateEmail = (email) => {
    if (!email || !/\S+@\S+\.\S+/.test(email)) {
      return "Email is invalid";
    }
    return null;
  };

  const validatePassword = (criteria) => {
    switch (false) {
      case criteria.length:
        return "Password must be at least 8 characters";
      case criteria.number:
        return "Password must contain a number";
      case criteria.special:
        return "Password must contain a special character";
      case criteria.uppercase:
        return "Password must contain an uppercase letter";
      case criteria.lowercase:
        return "Password must contain a lowercase letter";
      default:
        return null;
    }
  };

  const validate = () => {
    if (isEmail) {
      const emailError = validateEmail(value);
      setError(emailError);
      return !emailError;
    }
    
    if (isPassword) {
      const passwordError = validatePassword(passwordCriteria);
      setError(passwordError);
      return !passwordError;
    }
    
    return true;
  };

  const handleChange = (e) => {
    const newValue = e.target.value;
    setValue(newValue);
    
    if (error) {
      setError(null);
    }

    if (isEmail) {
      const emailError = validateEmail(newValue);
      setError(emailError);
    }
    
    if (isPassword) {
      const passwordError = validatePassword({
        length: newValue.length >= 8,
        number: /\d/.test(newValue),
        special: /[!@#$%^&*(),.?":{}|<>_+=\-~]/.test(newValue),
        uppercase: /[A-Z]/.test(newValue),
        lowercase: /[a-z]/.test(newValue),
      });
      setError(passwordError);
    }
  };

  return {
    value,
    setValue,
    error,
    setError,
    handleChange,
    validate,
    passwordCriteria,
  };
};