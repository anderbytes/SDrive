// Match Go strings.TrimSpace, including NEL and excluding the byte-order mark.
export const trimDriveName = (name: string) => name.replace(/^\p{White_Space}+|\p{White_Space}+$/gu, "");

export function driveNameError(name: string, fallback = ""): string {
  const value = trimDriveName(name) || fallback;
  if (!value || Array.from(value).length > 80)
    return "name must contain between 1 and 80 characters";
  if (/[\0\r\n]/.test(value)) return "name must not contain NUL or line breaks";
  return "";
}
