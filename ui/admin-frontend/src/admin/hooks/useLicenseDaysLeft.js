// Licensing has been removed from the system
// This hook is kept for backward compatibility but returns null values
const useLicenseDaysLeft = () => {
  return { 
    licenseDaysLeft: null, 
    loading: false, 
    error: null, 
    fetchLicenseDaysLeft: async () => null 
  };
};

export default useLicenseDaysLeft;
