-- Mesen の testrunner で emphasis.nes を動かし、LEFT で段を 8 → 0 と下げながら、色見本の真ん中の色 (RGB) を段ごとに出す。
--   Mesen.exe --testrunner emphasis.nes measure.lua
-- 1 行: 段、上の色見本 (その段の色と強調)、下の色見本 (分割が on なので常に強調)。行は青・赤・灰、列は $3x $2x $1x $0x。
local f = 0
local function onInput()
  emu.setInput({left = (f >= 60 and f % 20 == 0 and f <= 60 + 20 * 7)}, 0)
end
local function onFrame()
  f = f + 1
  if f >= 59 and (f - 59) % 20 == 0 then
    local buf = emu.getScreenBuffer()
    local s = "step " .. (8 - (f - 59) // 20)
    for half = 0, 1 do
      s = s .. (half == 0 and "  TOP" or "  BOT")
      for r = 0, 2 do
        s = s .. " |"
        for c = 0, 3 do
          local y = ((half == 0 and 6 or 18) + 2 * r) * 8 + 8
          local x = (8 + 4 * c) * 8 + 16
          s = s .. string.format(" %06x", buf[y * 256 + x + 1] & 0xffffff)
        end
      end
    end
    print(s)
  end
  if f >= 59 + 20 * 8 then emu.stop(0) end
end
emu.addEventCallback(onInput, emu.eventType.inputPolled)
emu.addEventCallback(onFrame, emu.eventType.endFrame)
