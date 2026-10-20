import { apiGet, apiPost, apiPut, apiDelete } from "@/lib/api-client";
import { withReasonQuery } from "@/lib/hris/delete-reason";
import type {
  PsikotesInstrument,
  PsikotesQuestion,
  InstrumentUpdatePayload,
  QuestionPayload,
} from "./types";

export const fetchInstruments = () =>
  apiGet<{ data: PsikotesInstrument[] }>("/api/psikotes/instruments").then((r) => r.data);

export const updateInstrument = (id: string, payload: InstrumentUpdatePayload) =>
  apiPut<{ data: PsikotesInstrument }>(`/api/psikotes/instruments/${id}`, payload).then(
    (r) => r.data
  );

export const fetchQuestions = (instrumentId: string) =>
  apiGet<{ data: PsikotesQuestion[] }>(`/api/psikotes/instruments/${instrumentId}/questions`).then(
    (r) => r.data
  );

export const createQuestion = (instrumentId: string, payload: QuestionPayload) =>
  apiPost<{ data: PsikotesQuestion }>(
    `/api/psikotes/instruments/${instrumentId}/questions`,
    payload
  ).then((r) => r.data);

export const updateQuestion = (id: string, payload: QuestionPayload) =>
  apiPut<{ data: PsikotesQuestion }>(`/api/psikotes/questions/${id}`, payload).then((r) => r.data);

export const deleteQuestion = (id: string, reason: string) =>
  apiDelete(withReasonQuery(`/api/psikotes/questions/${id}`, reason));
