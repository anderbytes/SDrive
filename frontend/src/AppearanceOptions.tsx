import { type AppearancePreferences } from "./appearance";
import { useLanguage } from "./i18n";
export function AppearanceOptions({value, onChange, section}: {value: AppearancePreferences; onChange: (p: AppearancePreferences) => void; section: "Appearance" | "Drive cards" | "Disconnection"}) {
 const {t} = useLanguage();
 const select = (key: "textSize" | "sidebar" | "motion" | "disconnectSize", label: string, options: [string,string][]) => <label>{t(label)}<select value={value[key]} onChange={e => onChange({...value, [key]: e.target.value})}>{options.map(([key,label]) => <option key={key} value={key}>{t(label)}</option>)}</select></label>;
 if (section === "Appearance") return <>
 {select("textSize", "Text size", [["small","Small"],["standard","Standard"],["large","Large"]])}
 {select("sidebar", "Sidebar", [["expanded","Expanded"],["collapsed","Collapsed"],["auto","Automatic"]])}
 {select("motion", "Animations", [["full","Full"],["reduced","Reduced"],["off","Disabled"]])}
 <p>{t("System reduced motion always takes precedence.")}</p></>;
 if (section === "Drive cards") return <>
 <fieldset><legend>{t("Visible card details")}</legend>{["Service", "Server and folder", "Mount destination", "Capacity"].map((label,i) => <label className="check" key={label}><input type="checkbox" checked={!(value.hiddenCardDetails & (1 << i))} onChange={() => onChange({...value, hiddenCardDetails: value.hiddenCardDetails ^ (1 << i)})}/>{t(label)}</label>)}</fieldset>
 <fieldset disabled={!!(value.hiddenCardDetails & 8)}><legend>{t("Capacity display")}</legend>{["Bar","Percentage","Sizes"].map((label,i) => <label className="check" key={label}><input type="checkbox" checked={!!(value.capacityParts & (1 << i))} disabled={value.capacityParts === (1 << i)} onChange={() => onChange({...value, capacityParts: value.capacityParts ^ (1 << i)})}/>{t(label)}</label>)}</fieldset>
 <p>{t("Name, status, actions and warnings always remain visible.")}</p></>;
 return <>{select("disconnectSize", "Disconnection notice size", [["compact","Compact"],["standard","Standard"],["large","Large"]])}
 <label>{t("Time before disappearing (ms)")}<input type="number" min="0" max="60000" step="1" required value={value.disconnectDelayMs} onChange={e => onChange({...value, disconnectDelayMs: e.target.valueAsNumber})}/></label>
 <p>{t("Only successful results disappear automatically. Zero hides success immediately; failures remain until dismissed. The notice stays in the bottom-right corner.")}</p></>;
}
