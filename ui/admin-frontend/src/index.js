import React from 'react';
import ReactDOM from 'react-dom/client';
import './index.css';
import App from './App';
import reportWebVitals from './reportWebVitals';

// Suppress console logs in production
if (process.env.NODE_ENV === 'production') {
  const noop = () => {};
  // Store original console methods
  const originalConsole = {
    log: console.log,
    warn: console.warn,
    info: console.info
  };
  
  // Replace with no-op in production
  console.log = noop;
  console.info = noop;
  console.warn = noop;
  
  // Keep error logging for critical issues
  // console.error remains unchanged
  
  // Add a method to restore original behavior if needed
  console.restoreConsole = () => {
    console.log = originalConsole.log;
    console.warn = originalConsole.warn;
    console.info = originalConsole.info;
  };
}

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);

// If you want to start measuring performance in your app, pass a function
// to log results (for example: reportWebVitals(console.log))
// or send to an analytics endpoint. Learn more: https://bit.ly/CRA-vitals
reportWebVitals();
