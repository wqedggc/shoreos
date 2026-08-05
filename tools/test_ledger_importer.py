#!/usr/bin/env python3
import importlib.util
import unittest
from pathlib import Path
from tempfile import TemporaryDirectory


SPEC = importlib.util.spec_from_file_location("ledger_importer", "tools/ledger_importer.py")
WORKER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(WORKER)


class LedgerImporterTest(unittest.TestCase):
    def test_source_type_falls_back_to_file_signature(self):
        with TemporaryDirectory() as directory:
            root = Path(directory)
            pdf = root / "mobile-upload"
            pdf.write_bytes(b"%PDF-1.7")
            self.assertEqual(WORKER.detect_source_type(pdf), "cmb_bank_pdf_text")
            xlsx = root / "document"
            xlsx.write_bytes(b"PK\x03\x04rest")
            self.assertEqual(WORKER.detect_source_type(xlsx), "wechat_xlsx")
            csv_file = root / "statement"
            csv_file.write_text("交易时间,交易分类", encoding="utf-8")
            self.assertEqual(WORKER.detect_source_type(csv_file), "alipay_csv")

    def test_wechat_header_location_is_not_fixed(self):
        rows = [("导出说明",), ("",), ("交易时间", "交易对方", "收/支", "金额(元)")]
        index, headers = WORKER.locate_wechat_header(rows)
        self.assertEqual(index, 2)
        self.assertEqual(headers[0], "交易时间")

    def test_order_identity_survives_status_changes(self):
        first = WORKER.transaction_fingerprint("alipay", "account", "order", "merchant", "first-row", 1)
        second = WORKER.transaction_fingerprint("alipay", "account", "order", "merchant", "updated-row", 1)
        self.assertEqual(first, second)

    def test_no_order_id_only_deduplicates_exact_source_row(self):
        first = WORKER.transaction_fingerprint("wechat", "account", "", "", "same-row", 1)
        second = WORKER.transaction_fingerprint("wechat", "account", "", "", "same-row", 2)
        self.assertNotEqual(first, second)


if __name__ == "__main__":
    unittest.main()
