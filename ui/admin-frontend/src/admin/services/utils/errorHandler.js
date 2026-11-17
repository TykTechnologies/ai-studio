// withContext keeps the HTTP status and the API's error code (errors[0].code)
// on the plain Error, so pages can tell responses with the same status apart.
const withContext = (out, error) => {
  out.status = error?.response?.status;
  out.code = error?.response?.data?.errors?.[0]?.code || '';
  return out;
};

export const handleApiError = (error) => {
  // Typed authorization errors carry meaning (permission vs. edition) that
  // pages act on; pass them through untouched.
  if (error?.isPermissionDenied || error?.isEnterpriseFeature) {
    return error;
  }
  if (error.response?.data?.errors && error.response.data.errors.length > 0) {
    const errorDetail = error.response.data.errors[0].detail;
    return withContext(new Error(errorDetail), error);
  } else if (error.response?.data?.message) {
    return withContext(new Error(error.response.data.message), error);
  } else if (error.response?.data?.error) {
    return withContext(new Error(error.response.data.error), error);
  } else if (error.message) {
    return withContext(new Error(error.message), error);
  } else {
    return withContext(new Error('Unknown error occurred'), error);
  }
};
