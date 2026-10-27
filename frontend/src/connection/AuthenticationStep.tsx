import { SecretInput } from "./SecretInput";
import type { Wizard } from "./useConnectionWizard";
import { LoadingStatus } from "../Spinner";

export function AuthenticationStep({ wizard }: { wizard: Wizard }) {
  const {
    t, sessionID, session, answer, setAnswer, busy, fieldID,
    question, answerQuestion,
  } = wizard;
  return (
    <div className="auth-flow">
      {!session || session.busy ? (
        <>
          <LoadingStatus>{t("Waiting for authentication…")}</LoadingStatus>
          <p>
            {t(
              "Complete sign-in in your browser if prompted, then return to SDrive.",
            )}
          </p>
        </>
      ) : question ? (
        question.Unsupported ? (
          <p role="alert">{t(question.Help)}</p>
        ) : (
          <form
            id={`${fieldID}-question`}
            onSubmit={(e) => {
              e.preventDefault();
              void answerQuestion();
            }}
          >
            {busy && <LoadingStatus>{t("Preparing…")}</LoadingStatus>}
            <h3>{t("Continue setup")}</h3>
            <p id={`${fieldID}-question-help`}>{t(question.Help)}</p>
            {question.IsPassword || question.Sensitive ? (
              <SecretInput key={`${sessionID}-${question.Name}-${session?.error || ""}`}
                label={t(question.Name)} value={answer} required={question.Required} describedBy={`${fieldID}-question-help`}
                onChange={setAnswer} />
            ) : <label>
              {t(question.Name)}
              {question.Exclusive && question.Examples?.length ? (
                <select
                  aria-label={t(question.Name)}
                  aria-describedby={`${fieldID}-question-help`}
                  required={question.Required}
                  value={answer}
                  onChange={(e) => setAnswer(e.target.value)}
                >
                  <option value="">{t("Select…")}</option>
                  {question.Examples.map((item) => (
                    <option key={item.Value} value={item.Value}>
                      {question.Name === "Account type" ||
                      question.Type === "bool"
                        ? t(item.Help || item.Value)
                        : item.Help || item.Value}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  aria-label={t(question.Name)}
                  aria-describedby={`${fieldID}-question-help`}
                  autoComplete="off"
                  type="text"
                  required={question.Required}
                  value={answer}
                  onChange={(e) => setAnswer(e.target.value)}
                />
              )}
            </label>}
          </form>
        )
      ) : (
        <p>{t("Access was not completed. Cancel and try again.")}</p>
      )}
    </div>
  );
}

export function AuthenticationActions({ wizard }: { wizard: Wizard }) {
  const { t, busy, close, question, fieldID, session } = wizard;
  return <>
    <button disabled={busy} onClick={close}>{t("Cancel setup")}</button>
    {session && !session.busy && question && !question.Unsupported && <button form={`${fieldID}-question`} disabled={busy} className="primary">{t("Continue")}</button>}
  </>;
}
