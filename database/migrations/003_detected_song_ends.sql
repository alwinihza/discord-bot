-- Song ends found by fionastation's scripts/song-ends batch, in seconds from the song's start.
-- Only that batch writes this column; timestamp_end stays the hand-entered end.
ALTER TABLE karaoke_song ADD COLUMN detected_end_s REAL;
UPDATE karaoke_song SET detected_end_s = 179.2 WHERE id = '03cbbc2e-d076-4ea0-95ca-5d78ef6f33cb';
UPDATE karaoke_song SET detected_end_s = 179.2 WHERE id = '17c9b235-c649-45a1-b33c-c3ecb2175357';
UPDATE karaoke_song SET detected_end_s = 206.8 WHERE id = '25933e53-e297-4a31-baca-2333017b57bf';
UPDATE karaoke_song SET detected_end_s = 316.8 WHERE id = '29eec7c8-68a4-4717-bed6-94aeb0ce1ee7';
UPDATE karaoke_song SET detected_end_s = 234.8 WHERE id = '2fe4c948-474e-4b7c-970e-f52cbc8a2824';
UPDATE karaoke_song SET detected_end_s = 244.2 WHERE id = '3357b828-a041-402c-a694-451807385333';
UPDATE karaoke_song SET detected_end_s = 172.2 WHERE id = '3772c315-9318-4791-b9c1-21da35d9ad9d';
UPDATE karaoke_song SET detected_end_s = 167.8 WHERE id = '3f4c7d92-0ae9-40a4-b160-a0e53844af54';
UPDATE karaoke_song SET detected_end_s = 171.2 WHERE id = '40a75aa0-9bb2-4fa5-906b-d88fd20e468b';
UPDATE karaoke_song SET detected_end_s = 231.2 WHERE id = '66e71771-6a85-47b1-acc1-adea80ab83c5';
UPDATE karaoke_song SET detected_end_s = 37.8 WHERE id = 'acaec333-f6dc-492d-8a87-55403a43953f';
UPDATE karaoke_song SET detected_end_s = 183.2 WHERE id = 'ba327fb1-b33b-49d8-93c1-c4f39751e9af';
UPDATE karaoke_song SET detected_end_s = 179.2 WHERE id = 'bfaec79b-b881-46a0-8e8e-5fb58e38d785';
UPDATE karaoke_song SET detected_end_s = 40.2 WHERE id = 'e2b28829-474d-4568-8bdc-730aa171e03f';
