export const formatBytes = (value: number | null) => {
  if (value === null) return "—";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  const index = value > 0 ? Math.min(Math.floor(Math.log2(value) / 10), units.length - 1) : 0;
  return `${(value / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`;
};
