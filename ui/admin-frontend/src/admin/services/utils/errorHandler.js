export const handleApiError = (error) => {
  if (error.response?.data?.errors && error.response.data.errors.length > 0) {
    const errorDetail = error.response.data.errors[0].detail;
    return new Error(errorDetail);
  } else if (error.response?.data?.message) {
    return new Error(error.response.data.message);
  } else if (error.message) {
    return new Error(error.message);
  } else {
    return new Error('Unknown error occurred');
  }
};