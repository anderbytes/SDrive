import {test,expect,afterEach} from "vitest";
import {render,screen,fireEvent,cleanup} from "@testing-library/react";
import {useState} from "react";
import {AppearanceOptions} from "./AppearanceOptions";
import {appearanceDefaults,applyAppearance} from "./appearance";
afterEach(cleanup);
test("card toggles preserve at least one capacity part",()=>{
 function Controls(){ const [value,onChange]=useState(appearanceDefaults); return <AppearanceOptions section="Drive cards" value={value} onChange={onChange}/>; }
 render(<Controls/>);
 fireEvent.click(screen.getByLabelText("Percentage")); fireEvent.click(screen.getByLabelText("Sizes"));
 expect((screen.getByLabelText("Bar") as HTMLInputElement).disabled).toBe(true);
 fireEvent.click(screen.getByLabelText("Capacity")); expect((screen.getByLabelText("Capacity") as HTMLInputElement).checked).toBe(false);
});
test("text and motion application supports defaults and explicit settings",()=>{
 applyAppearance({...appearanceDefaults,textSize:"large",motion:"off"}); expect(document.documentElement.style.getPropertyValue("--text-scale")).toBe("1.15"); expect(document.documentElement.dataset.motion).toBe("off");
 applyAppearance(); expect(document.documentElement.style.getPropertyValue("--text-scale")).toBe("1");
});
