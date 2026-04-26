{ Pascal sample }
program Sample;

{$mode objfpc}{$H+}

uses
  SysUtils, Classes;

type
  TCounter = class
  private
    FValue: Integer;
  public
    constructor Create;
    function Increment(N: Integer): Integer;
    property Value: Integer read FValue;
  end;

constructor TCounter.Create;
begin
  FValue := 0;
end;

function TCounter.Increment(N: Integer): Integer;
begin
  Inc(FValue, N);
  Result := FValue;
end;

var
  Counter: TCounter;
  Lines: TStringList;
  i: Integer;
  Line: string;
  Path: string;
begin
  Path := 'input.txt';
  if ParamCount >= 1 then
    Path := ParamStr(1);

  Counter := TCounter.Create;
  Lines := TStringList.Create;
  try
    Lines.LoadFromFile(Path);
    for i := 0 to Lines.Count - 1 do
    begin
      Line := Lines[i];
      if Length(Line) > 0 then
        Counter.Increment(Length(Line));
    end;
    WriteLn('total bytes: ', Counter.Value);
  finally
    Lines.Free;
    Counter.Free;
  end;
end.
