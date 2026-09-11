export const handleApiError = (error) => {
  // Typed authorization errors carry meaning (permission vs. edition) that
  // pages act on; pass them through untouched.
  if (error?.isPermissionDenied || error?.isEnterpriseFeature) {
    return error;
  }
  if (error.response?.data?.errors && error.response.data.errors.length > 0) {
    const errorDetail = error.response.data.errors[0].detail;
    return new Error(errorDetail);
  } else if (error.response?.data?.message) {
    return new Error(error.response.data.message);
  } else if (error.response?.data?.error) {
    return new Error(error.response.data.error);
  } else if (error.message) {
    return new Error(error.message);
  } else {
    return new Error('Unknown error occurred');
  }
};
