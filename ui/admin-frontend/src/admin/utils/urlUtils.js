export const getBaseUrl = () => {
  const isDev = process.env.NODE_ENV === "development";
  const host = window.location.host;
  const protocol = window.location.protocol;
  return `${protocol}//${host}`;
};
