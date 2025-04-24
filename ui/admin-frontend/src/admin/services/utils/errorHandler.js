export const handleApiError = (error) => {
  if (error.response?.data?.message) {
    return new Error(error.response.data.message);
  } else if (error.message) {
    return new Error(error.message);
  } else {
    return new Error('Unknown error occurred');
  }
};