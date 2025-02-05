export const getBaseUrl = () => {
  const isDev = process.env.NODE_ENV === "development";
  const host = isDev ? "localhost:8080" : window.location.host;
  const protocol = window.location.protocol;
  return `${protocol}//${host}`;
};
