export function generateRandomEmail() {
  const randomString = Math.random().toString(36).substring(2, 15);
  return `user_${randomString}@tyk.io`;
}
